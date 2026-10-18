package cluster

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// ProbeNode verifies the registered address and exact certificate before enabling work.
func (r *Runtime) ProbeNode(ctx context.Context, n Node) error {
	tlsConfig := r.tlsConfig.Clone()
	tlsConfig.ClientAuth = tls.NoClientCert
	tlsConfig.ServerName = n.ID
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return ErrForbidden
		}
		h := sha256.Sum256(state.PeerCertificates[0].Raw)
		if hex.EncodeToString(h[:]) != n.CertificateSHA256 {
			return ErrForbidden
		}
		return nil
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+n.Address+"/cluster/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ErrNotReady
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrNotReady
	}
	var body struct {
		NodeID string `json:"node_id"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil || body.NodeID != n.ID {
		return ErrNotReady
	}
	return nil
}
