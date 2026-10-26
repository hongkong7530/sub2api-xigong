//go:build linux

package cluster

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestClusterHAProxyProcess(t *testing.T) {
	binary := os.Getenv("SUB2API_CLUSTER_HAPROXY")
	if binary == "" {
		t.Skip("isolated Linux HAProxy binary required")
	}
	dir := t.TempDir()
	certFile, keyFile, caFile := testProxyIdentity(t, dir)
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("worker")) }))
	// Start with the registered identity; changing an already-serving TLS config would race handshakes.
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	require.NoError(t, err)
	backend.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13}
	backend.StartTLS()
	defer backend.Close()
	address := backend.Listener.Addr().String()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	cfgFile, pidFile, socket := filepath.Join(dir, "haproxy.cfg"), filepath.Join(dir, "pid"), filepath.Join(dir, "admin.sock")
	original := fmt.Sprintf("global\n    stats socket %s level admin\ndefaults\n    mode http\n    timeout connect 1s\n    timeout client 3s\n    timeout server 3s\nfrontend gateway\n    bind 127.0.0.1:%d\n    default_backend workers\n%s\nbackend workers\n%s\n", socket, port, managedBegin, managedEnd)
	require.NoError(t, os.WriteFile(cfgFile, []byte(original), 0600))
	cfg := config.ClusterConfig{HAProxyConfig: cfgFile, HAProxyBinary: binary, HAProxyService: "lab", CAFile: caFile, CertFile: certFile, KeyFile: keyFile, StateDir: dir}
	proxy := NewProxy(cfg, socket, "workers")
	start := func(ctx context.Context) error {
		args := []string{"-D", "-f", cfgFile, "-p", pidFile}
		if data, err := os.ReadFile(pidFile); err == nil {
			args = append(args, "-sf", strings.TrimSpace(string(data)))
		}
		out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("HAProxy start: %s: %w", out, err)
		}
		return nil
	}
	proxy.command = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" {
			return nil, start(ctx)
		}
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGTERM)
			}
		}
	})
	node := fixtureNode()
	node.Address = address
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, proxy.Apply(ctx, []Node{node}))
	require.Eventually(t, func() bool {
		states, err := proxy.States(ctx)
		return err == nil && strings.HasPrefix(states[node.ID].State, "UP")
	}, 5*time.Second, 50*time.Millisecond)
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	_ = response.Body.Close()
	_, err = proxy.Command(ctx, "set server workers/node-1 weight 3")
	require.NoError(t, err)
	states, err := proxy.States(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, states[node.ID].Weight)
	require.Equal(t, node.MaxInFlight, states[node.ID].MaxConnections)
	_, err = proxy.Command(ctx, "set maxconn server workers/node-1 2")
	require.NoError(t, err)
	states, err = proxy.States(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, states[node.ID].MaxConnections, "runtime capacity must be read back independently of the file")
	_, err = proxy.Command(ctx, fmt.Sprintf("set maxconn server workers/node-1 %d", node.MaxInFlight))
	require.NoError(t, err)
	// Restart must reconstruct configured membership and baseline weight, not retain memory-only edits.
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.NoError(t, syscall.Kill(pid, syscall.SIGTERM))
	require.Eventually(t, func() bool { _, err := proxy.States(ctx); return err != nil }, 3*time.Second, 25*time.Millisecond)
	require.NoError(t, os.Remove(pidFile))
	require.NoError(t, start(ctx))
	require.Eventually(t, func() bool { s, err := proxy.States(ctx); return err == nil && s[node.ID].Weight == node.BaseWeight }, 3*time.Second, 25*time.Millisecond)
	node.State = "paused"
	node.Revision++
	require.NoError(t, proxy.Apply(ctx, []Node{node}))
	require.Eventually(t, func() bool {
		s, err := proxy.States(ctx)
		return err == nil && strings.Contains(s[node.ID].State, "MAINT")
	}, 3*time.Second, 25*time.Millisecond)
	node.State = "retired"
	node.Revision++
	require.NoError(t, proxy.Apply(ctx, []Node{node}))
	require.Eventually(t, func() bool { s, err := proxy.States(ctx); _, exists := s[node.ID]; return err == nil && !exists }, 3*time.Second, 25*time.Millisecond)
}

func testProxyIdentity(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "node-1"}, DNSNames: []string{"node-1"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, pub, priv)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	certFile, keyFile, caFile := filepath.Join(dir, "node.pem"), filepath.Join(dir, "node.key"), filepath.Join(dir, "ca.pem")
	for name, data := range map[string][]byte{certFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyFile: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), caFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})} {
		require.NoError(t, os.WriteFile(name, data, 0600))
	}
	return certFile, keyFile, caFile
}
