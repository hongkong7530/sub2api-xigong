package cluster

import (
	"bufio"
	"context"
	"database/sql"
	"database/sql/driver"
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
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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
	// Proxy waiting must share the bounded admission budget, not inherit the long stream timeout.
	if p.cfg.QueueTimeoutMS > 0 {
		fmt.Fprintf(&b, "    timeout queue %dms\n", p.cfg.QueueTimeoutMS)
	}
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
	State          string
	Weight         int
	Active         int
	MaxConnections int
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
		limit, err := strconv.Atoi(fields[6])
		if err != nil {
			return nil, err
		}
		states[fields[1]] = ProxyState{State: fields[17], Weight: weight, Active: active, MaxConnections: limit}
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

// proxyRevision excludes acknowledgement fields so confirming an operation cannot invalidate itself.
func proxyRevision(nodes []Node) string {
	type revision struct {
		ID       string
		Revision int64
	}
	versions := make([]revision, 0, len(nodes))
	for _, n := range nodes {
		versions = append(versions, revision{n.ID, n.Revision})
	}
	slices.SortFunc(versions, func(a, b revision) int { return strings.Compare(a.ID, b.ID) })
	return fingerprint(versions)
}

func ingressProxiesApplied(nodes []Node, snapshots map[string]*Snapshot, revision string, now time.Time) bool {
	count := 0
	for _, n := range nodes {
		if n.State == "retired" || n.State == "pending" || (!slices.Contains(n.Roles, config.RoleIngress) && !slices.Contains(n.Roles, config.RoleController)) {
			continue
		}
		count++
		s := snapshots[n.ID]
		if s == nil || s.NodeID != n.ID || s.BootID == "" || !s.Fresh(now) || s.ObservedRevision != n.Revision || s.Proxy == nil {
			return false
		}
		p := s.Proxy
		if p.BootID != s.BootID || p.Error != "" || p.Revision != revision || !(Snapshot{At: p.At}).Fresh(now) {
			return false
		}
	}
	return count > 0
}

func releaseProxyLock(conn *sql.Conn, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var unlocked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, key).Scan(&unlocked); err != nil || !unlocked {
		// Close returns a session to the pool; an uncertain lock requires physical eviction.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
}

func (r *Runtime) observeProxy(revision, code string) {
	r.mu.Lock()
	r.proxyObservation = &ProxyObservation{BootID: r.bootID, At: time.Now().UTC(), Revision: revision, Error: code}
	r.mu.Unlock()
}

func (r *Runtime) reconcile(parent context.Context) {
	if r.proxy == nil || r.cfg.Cluster.Shadow {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	nodes, err := r.Store.Nodes(ctx)
	if err != nil {
		r.observeProxy("", "proxy_registry_read_failed")
		return
	}
	// Preserve the controller lock domain during rolling upgrades; other ingresses own only their host.
	lockKey := r.cfg.Cluster.ID + ":ingress"
	if !r.IsController() {
		lockKey += ":" + r.cfg.Cluster.NodeID
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		r.observeProxy("", "proxy_lock_connection_failed")
		return
	}
	defer conn.Close()
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, lockKey).Scan(&locked); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		r.observeProxy("", "proxy_lock_failed")
		return
	}
	if !locked {
		r.observeProxy("", "proxy_lock_busy")
		return
	}
	defer releaseProxyLock(conn, lockKey)
	if err = r.proxy.Apply(ctx, nodes); err != nil {
		r.markApplyError(ctx, conn, "proxy_configuration_failed")
		return
	}
	states, err := r.proxy.States(ctx)
	if err != nil {
		r.markApplyError(ctx, conn, "proxy_readback_failed")
		return
	}
	wanted := make(map[string]ProxyState)
	snapshots := make(map[string]*Snapshot)
	for _, n := range nodes {
		if n.State == "retired" || !slices.Contains(n.Roles, config.RoleWorker) {
			continue
		}
		actual, present := states[n.ID]
		if !present {
			r.markApplyError(ctx, conn, "proxy_member_missing")
			return
		}
		if actual.MaxConnections != n.MaxInFlight {
			if _, err = r.proxy.Command(ctx, fmt.Sprintf("set maxconn server %s/%s %d", r.proxy.backend, n.ID, n.MaxInFlight)); err != nil {
				r.markApplyError(ctx, conn, "proxy_capacity_failed")
				return
			}
		}
		snapshot, snapshotErr := r.Snapshot(ctx, n.ID)
		if snapshotErr != nil {
			snapshot = nil
		}
		snapshots[n.ID] = snapshot
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
		commands := []string{fmt.Sprintf("set server %s/%s weight %d", r.proxy.backend, n.ID, target)}
		stateCommand := fmt.Sprintf("set server %s/%s state %s", r.proxy.backend, n.ID, state)
		if weight == 0 {
			commands = append([]string{stateCommand}, commands...)
		} else {
			commands = append(commands, stateCommand)
		}
		for _, command := range commands {
			if _, err = r.proxy.Command(ctx, command); err != nil {
				r.markApplyError(ctx, conn, "proxy_runtime_update_failed")
				return
			}
		}
		wanted[n.ID] = ProxyState{State: state, Weight: target, MaxConnections: n.MaxInFlight}
	}
	readback, err := r.proxy.States(ctx)
	if err != nil {
		r.markApplyError(ctx, conn, "proxy_readback_failed")
		return
	}
	for _, n := range nodes {
		got, present := readback[n.ID]
		if n.State == "retired" {
			if present {
				r.markApplyError(ctx, conn, "proxy_retirement_unconfirmed")
				return
			}
			continue
		}
		want, required := wanted[n.ID]
		if !required {
			continue
		}
		stateOK := (want.State == "ready" && strings.HasPrefix(got.State, "UP")) || (want.State == "drain" && strings.Contains(got.State, "DRAIN")) || (want.State == "maint" && strings.Contains(got.State, "MAINT"))
		if !present || got.MaxConnections != want.MaxConnections || got.Weight != want.Weight || !stateOK {
			r.markApplyError(ctx, conn, "proxy_runtime_unconfirmed")
			return
		}
	}
	revision := proxyRevision(nodes)
	r.observeProxy(revision, "")
	r.lastProxyError = ""
	// Only the controller confirms operations, after every registered serving ingress read back this plan.
	if !r.IsController() {
		return
	}
	var ingressIDs []string
	for _, n := range nodes {
		if slices.Contains(n.Roles, config.RoleIngress) || slices.Contains(n.Roles, config.RoleController) {
			ingressIDs = append(ingressIDs, n.ID)
		}
	}
	ingressSnapshots, err := r.Snapshots(ctx, ingressIDs)
	if err != nil || !ingressProxiesApplied(nodes, ingressSnapshots, revision, time.Now()) {
		return
	}
	for _, n := range nodes {
		if n.State == "retired" {
			if _, err = conn.ExecContext(ctx, `UPDATE cluster_nodes SET applied_revision=$3,applied_at=NOW(),apply_error=NULL WHERE cluster_id=$1 AND node_id=$2 AND revision=$3 AND applied_revision<>$3`, r.cfg.Cluster.ID, n.ID, n.Revision); err == nil {
				_, err = conn.ExecContext(ctx, `UPDATE cluster_operations SET status='awaiting_revocation',error_code=NULL WHERE cluster_id=$1 AND node_id=$2 AND target_revision=$3 AND status IN ('pending','error')`, r.cfg.Cluster.ID, n.ID, n.Revision)
			}
		} else if slices.Contains(n.Roles, config.RoleWorker) {
			s := snapshots[n.ID]
			if s == nil || !s.Fresh(time.Now()) || s.ObservedRevision != n.Revision || (n.State == "ready" && !s.AdmissionReady) {
				continue
			}
			if _, err = conn.ExecContext(ctx, `UPDATE cluster_nodes SET applied_revision=$3,applied_at=NOW(),apply_error=NULL WHERE cluster_id=$1 AND node_id=$2 AND revision=$3 AND applied_revision<>$3`, r.cfg.Cluster.ID, n.ID, n.Revision); err == nil {
				_, err = conn.ExecContext(ctx, `UPDATE cluster_operations SET status='applied',applied_at=NOW(),error_code=NULL WHERE cluster_id=$1 AND node_id=$2 AND target_revision=$3 AND status IN ('pending','error')`, r.cfg.Cluster.ID, n.ID, n.Revision)
			}
		}
		if err != nil {
			r.markApplyError(ctx, conn, "proxy_confirmation_failed")
			return
		}
	}
}

func (r *Runtime) markApplyError(ctx context.Context, conn *sql.Conn, code string) {
	r.observeProxy("", code)
	if !r.IsController() {
		if r.lastProxyError != code {
			logger.LegacyPrintf("cluster.proxy", "local ingress proxy reconciliation failed on %s: %s", r.cfg.Cluster.NodeID, code)
		}
		r.lastProxyError = code
		return
	}
	_, _ = conn.ExecContext(ctx, `UPDATE cluster_nodes SET apply_error=$2 WHERE cluster_id=$1 AND applied_revision<>revision AND apply_error IS DISTINCT FROM $2`, r.cfg.Cluster.ID, code)
	_, _ = conn.ExecContext(ctx, `UPDATE cluster_operations SET status='error',error_code=$2 WHERE cluster_id=$1 AND status='pending'`, r.cfg.Cluster.ID, code)
}
