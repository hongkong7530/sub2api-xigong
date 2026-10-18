//go:build linux

package cluster

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const capacityWireLimit = int64(16 << 20)

type capacityMeter struct {
	remaining atomic.Int64
	used      atomic.Int64
}

func newCapacityMeter() *capacityMeter {
	m := &capacityMeter{}
	m.remaining.Store(capacityWireLimit)
	return m
}
func (m *capacityMeter) reserve(n int) int {
	for {
		left := m.remaining.Load()
		if left <= 0 {
			return 0
		}
		take := min(int64(n), left)
		if m.remaining.CompareAndSwap(left, left-take) {
			return int(take)
		}
	}
}

type capacityConn struct {
	net.Conn
	meter *capacityMeter
}

func (c *capacityConn) transfer(p []byte, write bool) (int, error) {
	n := c.meter.reserve(len(p))
	if n == 0 {
		return 0, fmt.Errorf("isolated capacity wire budget exhausted")
	}
	var used int
	var err error
	if write {
		used, err = c.Conn.Write(p[:n])
	} else {
		used, err = c.Conn.Read(p[:n])
	}
	c.meter.remaining.Add(int64(n - used))
	c.meter.used.Add(int64(used))
	return used, err
}
func (c *capacityConn) Read(p []byte) (int, error)  { return c.transfer(p, false) }
func (c *capacityConn) Write(p []byte) (int, error) { return c.transfer(p, true) }

type capacityListener struct {
	net.Listener
	meter *capacityMeter
}

func (l capacityListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	return &capacityConn{Conn: c, meter: l.meter}, nil
}
func capacityDial(m *capacityMeter) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, address)
		if e != nil {
			return nil, e
		}
		return &capacityConn{Conn: c, meter: m}, nil
	}
}

type capacityEndpoint struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}
type capacityRound struct {
	Name              string                     `json:"name"`
	Completed         int                        `json:"completed"`
	Failures          int                        `json:"failures"`
	ElapsedMS         float64                    `json:"elapsed_ms"`
	RequestsPerSecond float64                    `json:"requests_per_second"`
	P50MS             float64                    `json:"p50_ms"`
	P95MS             float64                    `json:"p95_ms"`
	Distribution      map[string]int             `json:"distribution"`
	Before            map[string]json.RawMessage `json:"before"`
	After             map[string]json.RawMessage `json:"after"`
}

// Explicit opt-in only. This measures Gate + generated HAProxy + mock upstream,
// not the full gateway, database throughput, model capacity, or production weights.
func TestClusterCapacityLab(t *testing.T) {
	mode := os.Getenv("SUB2API_CAPACITY_MODE")
	if mode == "" {
		t.Skip("explicit isolated three-host capacity authorization required")
	}
	dir := os.Getenv("SUB2API_CAPACITY_DIR")
	require.True(t, strings.HasPrefix(dir, "/var/tmp/sub2api-capacity-"))
	require.True(t, filepath.IsAbs(dir))
	info, err := os.Lstat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Zero(t, info.Mode()&os.ModeSymlink)
	switch mode {
	case "identity":
		capacityIdentity(t, dir)
	case "server":
		capacityServer(t, dir)
	case "driver":
		capacityDriver(t, dir)
	default:
		t.Fatal("invalid capacity mode")
	}
}
func capacityIdentity(t *testing.T, dir string) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated-capacity-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	for i, id := range []string{"xigong3", "xigong", "xigong2", "lab-client"} {
		p, k, e := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, e)
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: id}, DNSNames: []string{id}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		d, e := x509.CreateCertificate(rand.Reader, leaf, ca, p, key)
		require.NoError(t, e)
		kd, e := x509.MarshalPKCS8PrivateKey(k)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: d}), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}), 0600))
	}
}
func capacityTLS(t *testing.T, dir, id string) *tls.Config {
	cert, e := tls.LoadX509KeyPair(filepath.Join(dir, id+".pem"), filepath.Join(dir, id+".key"))
	require.NoError(t, e)
	ca, e := os.ReadFile(filepath.Join(dir, "ca.pem"))
	require.NoError(t, e)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(ca))
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: pool, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
}
func capacityServer(t *testing.T, dir string) {
	id := os.Getenv("SUB2API_CAPACITY_NODE")
	require.Contains(t, []string{"xigong3", "xigong", "xigong2"}, id)
	meter := newCapacityMeter()
	gate := NewGate(16, 8)
	gate.Configure(true, 16, 8)
	var served, peak atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
		block := make([]byte, 1024)
		for i := 0; i < 2048; i++ {
			sum := sha256.Sum256(block)
			copy(block, sum[:])
		}
		w.Header().Set("X-Lab-Node", id)
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < 4; i++ {
				_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 240)+"\n\n")
				w.(http.Flusher).Flush()
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"fixture":"`+strings.Repeat("x", 1000)+`"}`)
		}
	}))
	defer upstream.Close()
	u, e := url.Parse(upstream.URL)
	require.NoError(t, e)
	proxy := httputil.NewSingleHostReverseProxy(u)
	transport := &http.Transport{DialContext: capacityDial(meter), MaxIdleConnsPerHost: 16}
	defer transport.CloseIdleConnections()
	proxy.Transport = transport
	proxy.FlushInterval = -1
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].Subject.CommonName != "lab-client" {
			http.Error(w, "forbidden", 403)
			return
		}
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(200)
			return
		case "/metrics":
			var usage syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			active, queued, _ := gate.Counts()
			_ = json.NewEncoder(w).Encode(map[string]any{"node": id, "served": served.Load(), "peak_active": peak.Load(), "active": active, "queued": queued, "queue_wait_ms": gate.QueueWaitMS(), "metered_wire_bytes": meter.used.Load(), "cpu_user_seconds": float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6, "cpu_system_seconds": float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6, "max_rss_kib": usage.Maxrss, "heap_bytes": mem.HeapAlloc})
			return
		}
		if r.Method != "GET" || (r.URL.Path != "/json" && r.URL.Path != "/sse") {
			http.NotFound(w, r)
			return
		}
		if served.Add(1) > 2048 {
			http.Error(w, "request budget", 429)
			return
		}
		release, err := gate.Acquire(r.Context(), 500*time.Millisecond)
		if err != nil {
			http.Error(w, "capacity", 503)
			return
		}
		defer release()
		a, _, _ := gate.Counts()
		for {
			old := peak.Load()
			if int64(a) <= old || peak.CompareAndSwap(old, int64(a)) {
				break
			}
		}
		proxy.ServeHTTP(w, r)
	})
	ln, e := net.Listen("tcp", "0.0.0.0:59190")
	require.NoError(t, e)
	server := &http.Server{Handler: handler, TLSConfig: capacityTLS(t, dir, id), ReadHeaderTimeout: 2 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(capacityListener{Listener: ln, meter: meter}, "", "") }()
	fmt.Println("CAPACITY_READY", id)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	select {
	case <-ctx.Done():
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Minute):
	}
	gate.Configure(false, 16, 8)
	stop, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	require.NoError(t, server.Shutdown(stop))
	fmt.Println("CAPACITY_STOPPED", id, meter.used.Load())
}
func capacityDriver(t *testing.T, dir string) {
	data, e := os.ReadFile(filepath.Join(dir, "endpoints.json"))
	require.NoError(t, e)
	var endpoints []capacityEndpoint
	require.NoError(t, json.Unmarshal(data, &endpoints))
	require.Len(t, endpoints, 3)
	meter := newCapacityMeter()
	readers := map[string]*http.Client{}
	for _, ep := range endpoints {
		tc := capacityTLS(t, dir, "lab-client")
		tc.ServerName = ep.ID
		tr := &http.Transport{TLSClientConfig: tc, DialContext: capacityDial(meter), MaxIdleConnsPerHost: 2}
		defer tr.CloseIdleConnections()
		readers[ep.ID] = &http.Client{Transport: tr, Timeout: 3 * time.Second}
	}
	metrics := func() map[string]json.RawMessage {
		result := map[string]json.RawMessage{}
		for _, ep := range endpoints {
			resp, err := readers[ep.ID].Get("https://" + ep.Address + "/metrics")
			require.NoError(t, err)
			body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
			_ = resp.Body.Close()
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			require.True(t, json.Valid(body))
			result[ep.ID] = body
		}
		return result
	}
	cfgPath, pidPath, sock := filepath.Join(dir, "haproxy.cfg"), filepath.Join(dir, "haproxy.pid"), filepath.Join(dir, "admin.sock")
	initial := fmt.Sprintf("global\n    stats socket %s level admin\n    maxconn 64\n    nbthread 1\ndefaults\n    mode http\n    timeout connect 2s\n    timeout client 10s\n    timeout server 10s\nfrontend isolated\n    bind 127.0.0.1:59191\n    default_backend workers\n%s\nbackend workers\n%s\n", sock, managedBegin, managedEnd)
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0600))
	p := NewProxy(config.ClusterConfig{HAProxyConfig: cfgPath, HAProxyBinary: "/usr/sbin/haproxy", HAProxyService: "isolated", StateDir: dir, CAFile: filepath.Join(dir, "ca.pem"), CertFile: filepath.Join(dir, "lab-client.pem"), KeyFile: filepath.Join(dir, "lab-client.key")}, sock, "workers")
	p.command = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "systemctl" {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
		a := []string{"-D", "-f", cfgPath, "-p", pidPath}
		if old, e := os.ReadFile(pidPath); e == nil {
			a = append(a, "-sf", strings.TrimSpace(string(old)))
		}
		return exec.CommandContext(ctx, p.cfg.HAProxyBinary, a...).CombinedOutput()
	}
	t.Cleanup(func() {
		if raw, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGTERM)
			}
		}
	})
	var report []capacityRound
	for round := 0; round < 4; round++ {
		selected := endpoints
		if round < 3 {
			selected = endpoints[round : round+1]
		}
		nodes := []Node{}
		for _, ep := range selected {
			n := fixtureNode()
			n.ID = ep.ID
			n.Address = ep.Address
			n.BaseWeight = 1
			n.MaxInFlight = 16
			n.MaxQueue = 8
			raw, err := os.ReadFile(filepath.Join(dir, ep.ID+".pem"))
			require.NoError(t, err)
			b, _ := pem.Decode(raw)
			require.NotNil(t, b)
			digest := sha256.Sum256(b.Bytes)
			n.CertificateSHA256 = hex.EncodeToString(digest[:])
			nodes = append(nodes, n)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		require.NoError(t, p.Apply(ctx, nodes))
		require.Eventually(t, func() bool {
			states, err := p.States(ctx)
			if err != nil {
				return false
			}
			for _, n := range nodes {
				if !strings.HasPrefix(states[n.ID].State, "UP") {
					return false
				}
			}
			return true
		}, 5*time.Second, 50*time.Millisecond)
		name := "three-nodes"
		if round < 3 {
			name = "single-" + selected[0].ID
		}
		result := capacityRound{Name: name, Distribution: map[string]int{}, Before: metrics()}
		tr := &http.Transport{DialContext: capacityDial(meter), MaxConnsPerHost: 16, MaxIdleConnsPerHost: 16}
		client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
		var index atomic.Int64
		var lock sync.Mutex
		var wg sync.WaitGroup
		latency := []float64{}
		start := time.Now()
		for worker := 0; worker < 16; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					index := index.Add(1)
					if index > 1024 || ctx.Err() != nil {
						return
					}
					path := "/json"
					if index%2 == 0 {
						path = "/sse"
					}
					begin := time.Now()
					request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:59191"+path, nil)
					resp, err := client.Do(request)
					node := ""
					success := false
					if err == nil {
						body, e := io.ReadAll(io.LimitReader(resp.Body, 4096))
						_ = resp.Body.Close()
						node = resp.Header.Get("X-Lab-Node")
						success = e == nil && resp.StatusCode == 200 && len(body) > 900 && node != ""
					}
					lock.Lock()
					if success {
						result.Completed++
						result.Distribution[node]++
						latency = append(latency, float64(time.Since(begin).Microseconds())/1000)
					} else {
						result.Failures++
					}
					lock.Unlock()
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)
		cancel()
		tr.CloseIdleConnections()
		result.ElapsedMS = float64(elapsed.Microseconds()) / 1000
		result.RequestsPerSecond = float64(result.Completed) / elapsed.Seconds()
		slices.Sort(latency)
		if len(latency) > 0 {
			result.P50MS = latency[(len(latency)-1)/2]
			result.P95MS = latency[(len(latency)-1)*95/100]
		}
		result.After = metrics()
		report = append(report, result)
	}
	encoded, e := json.MarshalIndent(map[string]any{"scope": "Gate + generated HAProxy + isolated mock upstream; no business DB", "concurrency": 16, "requests_per_round": 1024, "round_deadline_seconds": 60, "process_wire_cap_bytes": capacityWireLimit, "driver_metered_bytes": meter.used.Load(), "rounds": report}, "", "  ")
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), encoded, 0600))
	fmt.Println("CAPACITY_REPORT", string(encoded))
	for _, r := range report {
		require.Zero(t, r.Failures, r.Name)
		require.Equal(t, 1024, r.Completed, r.Name)
	}
}
