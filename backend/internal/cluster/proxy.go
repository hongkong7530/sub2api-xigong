package cluster

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const managedBegin = "# BEGIN SUB2API CLUSTER V3"
const managedEnd = "# END SUB2API CLUSTER V3"

type Proxy struct {
	cfg            config.ClusterConfig
	socket         string
	backend        string
	mu             sync.Mutex
	failedRevision string
	command        func(context.Context, string, ...string) ([]byte, error)
}

func NewProxy(cfg config.ClusterConfig, socket, backend string) *Proxy {
	return &Proxy{cfg: cfg, socket: socket, backend: backend, command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}
}
func haproxyPath(path string) (string, error) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\n\r\x00\"\\$") {
		return "", fmt.Errorf("unsafe HAProxy file path")
	}
	return `"` + path + `"`, nil
}

// Render replaces only the operator-approved managed block. Other listeners and backends survive.
func (p *Proxy) Render(original string, nodes []Node) (string, error) {
	if !identityPattern.MatchString(p.backend) {
		return "", fmt.Errorf("invalid managed backend")
	}
	if strings.Count(original, managedBegin) != 1 || strings.Count(original, managedEnd) != 1 {
		return "", fmt.Errorf("HAProxy configuration requires exactly one approved managed block")
	}
	start, end := strings.Index(original, managedBegin), strings.Index(original, managedEnd)
	if end < start {
		return "", fmt.Errorf("invalid HAProxy managed block order")
	}
	ca, err := haproxyPath(p.cfg.CAFile)
	if err != nil {
		return "", err
	}
	client, err := haproxyPath(path.Join(p.cfg.StateDir, "haproxy-client.pem"))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nbackend %s\n    mode http\n    balance leastconn\n    retries 0\n    option httpchk GET /health\n    http-check expect status 200\n", managedBegin, p.backend)
	ordered := append([]Node(nil), nodes...)
	slices.SortFunc(ordered, func(a, b Node) int { return strings.Compare(a.ID, b.ID) })
	for _, n := range ordered {
		if n.State == "retired" || !slices.Contains(n.Roles, "worker") {
			continue
		}
		if err := n.Validate(); err != nil {
			return "", err
		}
		disabled := ""
		if n.State != "ready" {
			disabled = " disabled"
		}
		fmt.Fprintf(&b, "    server %s %s ssl verify required verifyhost %s ca-file %s crt %s check check-sni %s sni str(%s) weight %d maxconn %d%s\n", n.ID, n.Address, n.ID, ca, client, n.ID, n.ID, n.BaseWeight, n.MaxInFlight, disabled)
	}
	b.WriteString(managedEnd)
	return original[:start] + b.String() + original[end+len(managedEnd):], nil
}
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cluster-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func (p *Proxy) Apply(ctx context.Context, nodes []Node) error {
	if p.cfg.Shadow {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	original, err := os.ReadFile(p.cfg.HAProxyConfig)
	if err != nil {
		return err
	}
	rendered, err := p.Render(string(original), nodes)
	if err != nil {
		return err
	}
	if rendered == string(original) {
		return nil
	}
	revision := fingerprint(nodes)
	if revision == p.failedRevision {
		return fmt.Errorf("previous configuration application failed; change or reapply its revision")
	}
	if err = os.MkdirAll(p.cfg.StateDir, 0700); err != nil {
		return err
	}
	cert, err := os.ReadFile(p.cfg.CertFile)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(p.cfg.KeyFile)
	if err != nil {
		return err
	}
	if err = writeAtomic(filepath.Join(p.cfg.StateDir, "haproxy-client.pem"), append(append(cert, '\n'), key...), 0600); err != nil {
		return err
	}
	candidate, err := os.CreateTemp(filepath.Dir(p.cfg.HAProxyConfig), ".cluster-validate-*")
	if err != nil {
		return err
	}
	name := candidate.Name()
	defer os.Remove(name)
	if err = candidate.Chmod(0600); err == nil {
		_, err = candidate.WriteString(rendered)
	}
	closeErr := candidate.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if _, err = p.command(ctx, p.cfg.HAProxyBinary, "-c", "-f", name); err != nil {
		p.failedRevision = revision
		return fmt.Errorf("HAProxy candidate validation failed: %w", err)
	}
	if err = writeAtomic(filepath.Join(p.cfg.StateDir, "haproxy.last-good.cfg"), original, 0600); err != nil {
		return err
	}
	if err = writeAtomic(p.cfg.HAProxyConfig, []byte(rendered), 0600); err != nil {
		return err
	}
	if _, err = p.command(ctx, "systemctl", "reload", p.cfg.HAProxyService); err != nil {
		p.failedRevision = revision
		if restoreErr := writeAtomic(p.cfg.HAProxyConfig, original, 0600); restoreErr != nil {
			return fmt.Errorf("reload and configuration restoration failed: %w", restoreErr)
		}
		return fmt.Errorf("HAProxy reload failed; persisted last-good configuration restored: %w", err)
	}
	p.failedRevision = ""
	return nil
}

type ProxyState struct {
	State  string
	Weight int
	Active int
}

func (p *Proxy) Command(ctx context.Context, command string) (string, error) {
	if p.cfg.Shadow {
		return "", fmt.Errorf("shadow controller has no runtime write access")
	}
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", p.socket)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(conn, command+"\n"); err != nil {
		return "", err
	}
	if u, ok := conn.(*net.UnixConn); ok {
		_ = u.CloseWrite()
	}
	data, err := io.ReadAll(io.LimitReader(conn, 2<<20))
	if err != nil {
		return "", err
	}
	out := string(data)
	if !strings.HasPrefix(command, "show ") && strings.TrimSpace(out) != "" {
		return "", fmt.Errorf("HAProxy rejected runtime command")
	}
	return out, nil
}
func (p *Proxy) States(ctx context.Context) (map[string]ProxyState, error) {
	out, err := p.Command(ctx, "show stat")
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(bufio.NewReader(strings.NewReader(out)))
	reader.FieldsPerRecord = -1
	states := map[string]ProxyState{}
	for {
		fields, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(fields) < 19 || fields[0] != p.backend || fields[1] == "BACKEND" {
			continue
		}
		weight, err := strconv.Atoi(fields[18])
		if err != nil {
			return nil, err
		}
		active, err := strconv.Atoi(fields[4])
		if err != nil {
			return nil, err
		}
		states[fields[1]] = ProxyState{State: fields[17], Weight: weight, Active: active}
	}
	return states, nil
}
func DesiredWeight(node Node, snapshot *Snapshot, now time.Time) int {
	if node.State != "ready" {
		return 0
	}
	if snapshot == nil || !snapshot.Fresh(now) {
		return node.BaseWeight
	}
	if !snapshot.DBOK || !snapshot.RedisOK || snapshot.PendingSettlement >= max(128, node.MaxInFlight*4) {
		return 0
	}
	if node.MonthlyLimitBytes != nil && snapshot.CycleTrafficBytes != nil && *snapshot.CycleTrafficBytes >= *node.MonthlyLimitBytes-node.TrafficReserveBytes {
		return 0
	}
	if node.MonthlyLimitBytes != nil && snapshot.CycleTrafficBytes != nil && *node.MonthlyLimitBytes > 0 {
		remaining := *node.MonthlyLimitBytes - *snapshot.CycleTrafficBytes - node.TrafficReserveBytes
		if remaining > 0 && remaining < max(node.TrafficReserveBytes, *node.MonthlyLimitBytes/10) {
			return max(1, node.BaseWeight/2)
		}
	}
	if snapshot.Active >= node.MaxInFlight || snapshot.Waiting > 0 || snapshot.QueueWaitMS > 250 {
		return max(1, node.BaseWeight/2)
	}
	return node.BaseWeight
}
func (r *Runtime) reconcile(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	nodes, err := r.Store.Nodes(ctx)
	if err != nil {
		return
	}
	if r.cfg.Cluster.Shadow {
		return
	}
	// A session-scoped lock prevents two live controllers from writing the same ingress.
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	var locked bool
	if conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, r.cfg.Cluster.ID+":ingress").Scan(&locked) != nil || !locked {
		return
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, r.cfg.Cluster.ID+":ingress")
	}()
	if err = r.proxy.Apply(ctx, nodes); err != nil {
		r.markApplyError(ctx, "proxy_configuration_failed")
		return
	}
	states, err := r.proxy.States(ctx)
	if err != nil {
		r.markApplyError(ctx, "proxy_readback_failed")
		return
	}
	for _, n := range nodes {
		actual, present := states[n.ID]
		if n.State == "retired" {
			if !present {
				_, _ = r.db.ExecContext(ctx, `UPDATE cluster_nodes SET applied_revision=$3,applied_at=NOW(),apply_error=NULL WHERE cluster_id=$1 AND node_id=$2 AND revision=$3 AND applied_revision<>$3`, r.cfg.Cluster.ID, n.ID, n.Revision)
				_, _ = r.db.ExecContext(ctx, `UPDATE cluster_operations SET status='awaiting_revocation',error_code=NULL WHERE cluster_id=$1 AND node_id=$2 AND target_revision=$3 AND status IN ('pending','error')`, r.cfg.Cluster.ID, n.ID, n.Revision)
			}
			continue
		}
		if !slices.Contains(n.Roles, "worker") {
			continue
		}
		if !present {
			continue
		}
		snapshot, snapshotErr := r.Snapshot(ctx, n.ID)
		if snapshotErr != nil {
			snapshot = nil
		}
		weight := DesiredWeight(n, snapshot, time.Now())
		target := weight
		if weight > 0 {
			step := max(1, n.BaseWeight/4)
			if actual.Weight <= 0 {
				target = min(weight, step)
			} else if weight > actual.Weight {
				target = min(weight, actual.Weight+step)
			} else {
				target = max(weight, actual.Weight-step)
			}
		}
		state := "ready"
		if weight == 0 {
			state = "drain"
		}
		if weight == 0 && n.State != "ready" {
			state = "maint"
		}
		if weight == 0 {
			if _, err = r.proxy.Command(ctx, fmt.Sprintf("set server %s/%s state %s", r.proxy.backend, n.ID, state)); err != nil {
				continue
			}
		}
		if _, err = r.proxy.Command(ctx, fmt.Sprintf("set server %s/%s weight %d", r.proxy.backend, n.ID, target)); err != nil {
			continue
		}
		if state == "ready" {
			if _, err = r.proxy.Command(ctx, fmt.Sprintf("set server %s/%s state ready", r.proxy.backend, n.ID)); err != nil {
				continue
			}
		}
		if n.State == "ready" && snapshot != nil && snapshot.Fresh(time.Now()) && !snapshot.AdmissionReady {
			continue
		}
		readback, err := r.proxy.States(ctx)
		if err != nil {
			continue
		}
		got, ok := readback[n.ID]
		if !ok || got.Weight != target || snapshot == nil || !snapshot.Fresh(time.Now()) || snapshot.ObservedRevision != n.Revision {
			continue
		}
		if state == "ready" && !strings.HasPrefix(got.State, "UP") {
			continue
		}
		if state != "ready" && !strings.Contains(got.State, "DRAIN") && !strings.Contains(got.State, "MAINT") {
			continue
		}
		_, _ = r.db.ExecContext(ctx, `UPDATE cluster_nodes SET applied_revision=$3,applied_at=NOW(),apply_error=NULL WHERE cluster_id=$1 AND node_id=$2 AND revision=$3 AND applied_revision<>$3`, r.cfg.Cluster.ID, n.ID, n.Revision)
		_, _ = r.db.ExecContext(ctx, `UPDATE cluster_operations SET status='applied',applied_at=NOW(),error_code=NULL WHERE cluster_id=$1 AND node_id=$2 AND target_revision=$3 AND status IN ('pending','error')`, r.cfg.Cluster.ID, n.ID, n.Revision)
	}
}
func (r *Runtime) markApplyError(ctx context.Context, code string) {
	_, _ = r.db.ExecContext(ctx, `UPDATE cluster_nodes SET apply_error=$2 WHERE cluster_id=$1 AND applied_revision<>revision AND apply_error IS DISTINCT FROM $2`, r.cfg.Cluster.ID, code)
	_, _ = r.db.ExecContext(ctx, `UPDATE cluster_operations SET status='error',error_code=$2 WHERE cluster_id=$1 AND status='pending'`, r.cfg.Cluster.ID, code)
}
