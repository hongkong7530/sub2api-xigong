//go:build linux

package cluster

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestClusterRuntimeEnrollmentTLSAndPause(t *testing.T) {
	dsn, socket := os.Getenv("SUB2API_CLUSTER_TEST_DSN"), os.Getenv("SUB2API_CLUSTER_REDIS_SOCKET")
	if dsn == "" || socket == "" {
		t.Skip("isolated PostgreSQL and Redis required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	rdb := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id := "tls-" + uuid.NewString()
	dir := t.TempDir()
	_, _, caPath := testProxyIdentity(t, dir)
	caPEM, err := os.ReadFile(caPath)
	require.NoError(t, err)
	caBlock, _ := pem.Decode(caPEM)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)
	keyPEM, err := os.ReadFile(filepath.Join(dir, "node.key"))
	require.NoError(t, err)
	keyBlock, _ := pem.Decode(keyPEM)
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	require.NoError(t, err)
	makeRuntime := func(name string, roles []string) (*Runtime, *http.Client, string) {
		pub, private, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		cert := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, key)
		require.NoError(t, err)
		privateDER, err := x509.MarshalPKCS8PrivateKey(private)
		require.NoError(t, err)
		certPath, keyPath := filepath.Join(dir, name+".pem"), filepath.Join(dir, name+".key")
		require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
		require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0600))
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		address := listener.Addr().String()
		require.NoError(t, listener.Close())
		cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: id, NodeID: name, Roles: roles, Capabilities: []string{"http"}, Shadow: true, ListenAddress: address, AdvertiseAddress: address, CAFile: caPath, CertFile: certPath, KeyFile: keyPath, StateDir: filepath.Join(dir, name), MaxInFlight: 2, MaxQueue: 1, QueueTimeoutMS: 500, DBConnectionBudget: 32, ShutdownSeconds: 10, ApplicationVersion: "fixture"}}
		runtime, err := NewRuntime(cfg, db, rdb)
		require.NoError(t, err)
		runtime.SetSettlementRecovery(func(context.Context) (int, error) { return 0, nil })
		hash := sha256.Sum256(der)
		n := fixtureNode()
		n.ID = name
		n.Name = name
		n.Address = address
		n.Roles = roles
		n.CertificateSHA256 = hex.EncodeToString(hash[:])
		n.Version = "fixture"
		n.MaxInFlight = 2
		token, err := runtime.Store.Register(ctx, n, 1)
		require.NoError(t, err)
		transport := &http.Transport{TLSClientConfig: runtime.tlsConfig.Clone(), ForceAttemptHTTP2: true}
		t.Cleanup(transport.CloseIdleConnections)
		return runtime, &http.Client{Transport: transport, Timeout: 3 * time.Second}, token
	}
	controller, controllerClient, controllerToken := makeRuntime("controller", []string{"ingress", "controller"})
	worker, workerClient, workerToken := makeRuntime("worker", []string{"worker"})
	t.Cleanup(func() {
		for _, table := range []string{"cluster_operations", "cluster_join_tokens", "cluster_traffic_counters", "cluster_node_credentials", "cluster_nodes", "cluster_control"} {
			_, _ = db.Exec(`DELETE FROM `+table+` WHERE cluster_id=$1`, id)
		}
	})
	streamDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		release, err := worker.Gate.Acquire(req.Context(), time.Second)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		defer release()
		if req.URL.Path == "/v1/stream-fixture" {
			defer close(streamDone)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: ready\n\n"))
			w.(http.Flusher).Flush()
			<-req.Context().Done()
			return
		}
		w.WriteHeader(204)
	})
	require.NoError(t, controller.Start(ctx, handler))
	require.NoError(t, worker.Start(ctx, handler))
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = worker.Stop(stop)
		_ = controller.Stop(stop)
	})
	join := func(client *http.Client, token string) int {
		body, _ := json.Marshal(map[string]string{"token": token})
		resp, err := client.Post("https://"+controller.cfg.Cluster.ListenAddress+"/cluster/enroll", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	require.Equal(t, 200, join(controllerClient, controllerToken))
	require.Equal(t, 200, join(workerClient, workerToken))
	require.Equal(t, 403, join(workerClient, workerToken))
	_, err = db.ExecContext(ctx, `UPDATE cluster_control SET settlement_mode='atomic' WHERE cluster_id=$1`, id)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		generation, _, err := controller.Store.Generation(ctx)
		return err == nil && generation != ""
	}, 5*time.Second, 25*time.Millisecond)
	generation, _, err := controller.Store.Generation(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return controller.AcknowledgeRedisRecovery(ctx, generation, 1) == nil }, 5*time.Second, 25*time.Millisecond)
	require.Eventually(t, func() bool {
		s, err := worker.Snapshot(ctx, "worker")
		return err == nil && s != nil && s.DBOK && s.RedisOK
	}, 5*time.Second, 25*time.Millisecond)
	op, err := controller.Change(ctx, "worker", Change{RequestKey: uuid.NewString(), ExpectedRevision: 1, Action: "activate"}, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), op.TargetRevision)
	require.Eventually(t, worker.Gate.Ready, 7*time.Second, 25*time.Millisecond)
	request := func(client *http.Client, path string) int {
		resp, err := client.Get("https://" + worker.cfg.Cluster.ListenAddress + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 2, resp.ProtoMajor, "node transport must negotiate HTTP/2")
		return resp.StatusCode
	}
	require.Equal(t, 204, request(controllerClient, "/v1/probe"))
	require.Equal(t, 403, request(workerClient, "/v1/probe"))
	// Streaming lifetime is bounded by the test context, not the short request timeout.
	controllerClient.Timeout = 0
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	streamRequest, err := http.NewRequestWithContext(streamCtx, http.MethodGet, "https://"+worker.cfg.Cluster.ListenAddress+"/v1/stream-fixture", nil)
	require.NoError(t, err)
	streamResponse, err := controllerClient.Do(streamRequest)
	require.NoError(t, err)
	defer streamResponse.Body.Close()
	require.Equal(t, 2, streamResponse.ProtoMajor)
	line, err := bufio.NewReader(streamResponse.Body).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: ready\n", line)
	_, err = controller.Change(ctx, "worker", Change{RequestKey: uuid.NewString(), ExpectedRevision: 2, Action: "pause"}, 1)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !worker.Gate.Ready() }, 7*time.Second, 25*time.Millisecond)
	require.Equal(t, 503, request(controllerClient, "/v1/probe"))
	active, _, _ := worker.Gate.Counts()
	require.Equal(t, 1, active, "pause must retain the existing SSE stream")
	cancelStream()
	select {
	case <-streamDone:
	case <-ctx.Done():
		t.Fatal("stream cancellation did not reach the node")
	}
	require.Eventually(t, func() bool { active, _, _ := worker.Gate.Counts(); return active == 0 }, time.Second, 10*time.Millisecond)
	worker.Pause()
	time.Sleep(2100 * time.Millisecond)
	require.False(t, worker.Gate.Ready(), "shutdown hold must survive another telemetry cycle")
}
