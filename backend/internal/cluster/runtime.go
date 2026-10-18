package cluster

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type Runtime struct {
	recoveryHealthy       atomic.Bool
	recoveryDone          chan struct{}
	reserve               *RecoveryReserve
	Store                 *Store
	Gate                  *Gate
	cfg                   *config.Config
	db                    *sql.DB
	redis                 *redis.Client
	bootID                string
	certificate           string
	tlsConfig             *tls.Config
	server                *http.Server
	mu                    sync.RWMutex
	local                 *Node
	lastConfig            time.Time
	cancel                context.CancelFunc
	done                  chan struct{}
	proxy                 *Proxy
	recoverSettlements    func(context.Context) (int, error)
	stopping              atomic.Bool
	lastTrafficCheckpoint time.Time
	trafficBootID         string
	// mismatch explains why this node is withheld from admission, so a silent version,
	// role or capability drift cannot masquerade as an ordinary 503.
	mismatch     string
	lastMismatch string
}

// NewRuntime validates TLS material without opening listeners or changing membership.
func NewRuntime(cfg *config.Config, db *sql.DB, rdb *redis.Client) (*Runtime, error) {
	r := &Runtime{cfg: cfg, db: db, redis: rdb, bootID: uuid.NewString(), done: make(chan struct{})}
	if !cfg.ClusterEnabled() {
		return r, nil
	}
	r.Store = &Store{DB: db, ClusterID: cfg.Cluster.ID, ConnectionBudget: cfg.Cluster.DBConnectionBudget}
	r.Gate = NewGate(cfg.Cluster.MaxInFlight, cfg.Cluster.MaxQueue)
	reserve, err := NewRecoveryReserve(cfg.Cluster.StateDir, max(128, cfg.Cluster.MaxInFlight*4))
	if err != nil {
		return nil, err
	}
	r.reserve = reserve
	if boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		r.trafficBootID = strings.TrimSpace(string(boot))
	}
	cert, err := tls.LoadX509KeyPair(cfg.Cluster.CertFile, cfg.Cluster.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("cluster node certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	if leaf.Subject.CommonName != cfg.Cluster.NodeID {
		return nil, fmt.Errorf("cluster certificate identity does not match node_id")
	}
	ca, err := os.ReadFile(cfg.Cluster.CAFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("invalid cluster CA")
	}
	h := sha256.Sum256(cert.Certificate[0])
	r.certificate = hex.EncodeToString(h[:])
	r.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: pool, RootCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
	if cfg.RunsRole(config.RoleController) {
		r.proxy = NewProxy(cfg.Cluster, cfg.Cluster.HAProxyRuntimeSocket, cfg.Cluster.ClusterBackendName)
	}
	return r, nil
}

func (r *Runtime) Enabled() bool      { return r != nil && r.cfg.ClusterEnabled() }
func (r *Runtime) IsController() bool { return r.Enabled() && r.cfg.RunsRole(config.RoleController) }
func (r *Runtime) Shadow() bool       { return r.Enabled() && r.cfg.Cluster.Shadow }
func (r *Runtime) Capability(name string) bool {
	return !r.Enabled() || r.cfg.HasClusterCapability(name)
}
func (r *Runtime) ControlURL() string      { return r.cfg.Cluster.ControlURL }
func (r *Runtime) RetentionApproved() bool { return r.Enabled() && r.cfg.Cluster.RetentionApproved }
func (r *Runtime) NodeID() string {
	if !r.Enabled() {
		return ""
	}
	return r.cfg.Cluster.NodeID
}
func (r *Runtime) SetRecoveryReserve(reserve *RecoveryReserve) {
	if reserve != nil {
		r.reserve = reserve
	}
}
func (r *Runtime) SetSettlementRecovery(recover func(context.Context) (int, error)) {
	r.recoverSettlements = recover
}
func (r *Runtime) Start(ctx context.Context, application http.Handler) error {
	if !r.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return nil
	}
	listener, err := net.Listen("tcp", r.cfg.Cluster.ListenAddress)
	if err != nil {
		return err
	}
	ctx, r.cancel = context.WithCancel(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cluster/health", r.health)
	mux.HandleFunc("POST /cluster/enroll", r.enroll)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		id, cert, err := r.peer(req)
		if err != nil {
			http.Error(w, "forbidden", 403)
			return
		}
		node, err := r.Store.Authenticate(req.Context(), id, cert)
		if err != nil || (!slices.Contains(node.Roles, "ingress") && !slices.Contains(node.Roles, "controller")) {
			http.Error(w, "forbidden", 403)
			return
		}
		if req.URL.Path != "/health" && !IsGatewayPath(req.URL.Path) {
			http.NotFound(w, req)
			return
		}
		if req.URL.Path == "/health" && !r.Gate.Ready() {
			http.Error(w, "not ready", 503)
			return
		}
		application.ServeHTTP(w, req)
	})
	// ServeTLS negotiates HTTP/2 without weakening the node's mutual TLS policy.
	r.server = &http.Server{Handler: mux, TLSConfig: r.tlsConfig, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 8192}
	go func() { _ = r.server.ServeTLS(listener, "", "") }()
	r.recoveryDone = make(chan struct{})
	go r.recoveryLoop(ctx)
	go r.loop(ctx)
	return nil
}
func (r *Runtime) Pause() {
	if r.Enabled() {
		r.stopping.Store(true)
		r.Gate.Configure(false, r.cfg.Cluster.MaxInFlight, r.cfg.Cluster.MaxQueue)
	}
}
func (r *Runtime) Stop(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	r.Pause()
	r.mu.Lock()
	cancel := r.cancel
	server := r.server
	r.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if server != nil {
		_ = server.Shutdown(ctx)
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *Runtime) WaitIdle(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		a, w, p := r.Gate.Counts()
		if a == 0 && w == 0 && p == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (r *Runtime) snapshotKey(id string) string {
	return "cluster:v3:" + r.cfg.Cluster.ID + ":node:" + id
}
func (r *Runtime) Snapshot(ctx context.Context, id string) (*Snapshot, error) {
	if r.redis == nil {
		return nil, ErrNotReady
	}
	data, err := r.redis.Get(ctx, r.snapshotKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot Snapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}
func (r *Runtime) NodeViews(ctx context.Context) ([]map[string]any, error) {
	nodes, err := r.Store.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		snapshot, err := r.Snapshot(ctx, n.ID)
		if err != nil {
			snapshot = nil
		}
		fresh := snapshot != nil && snapshot.Fresh(time.Now())
		out = append(out, map[string]any{"config": n, "status": snapshot, "fresh": fresh, "applied_revision": n.AppliedRevision, "suggested_weight": DesiredWeight(n, snapshot, time.Now())})
	}
	return out, nil
}
func (r *Runtime) Change(ctx context.Context, id string, c Change, actor int64) (*Operation, error) {
	if !r.Enabled() || !r.cfg.RunsRole(config.RoleController) {
		return nil, ErrForbidden
	}
	snapshot, err := r.Snapshot(ctx, id)
	if err != nil && c.Action != "pause" && c.Action != "force_retire" {
		return nil, err
	}
	if c.Action == "activate" || c.Action == "resume" {
		n, err := r.Store.Node(ctx, id)
		if err != nil {
			return nil, err
		}
		if err = r.ProbeNode(ctx, *n); err != nil {
			return nil, err
		}
	}
	return r.Store.Change(ctx, id, c, actor, snapshot)
}
func (r *Runtime) loop(ctx context.Context) {
	defer close(r.done)
	defer func() {
		if r.recoveryDone != nil {
			<-r.recoveryDone
		}
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		r.sample(ctx)
		if r.proxy != nil {
			r.reconcile(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (r *Runtime) sample(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
	defer cancel()
	now := time.Now().UTC()
	r.mu.RLock()
	node := r.local
	last := r.lastConfig
	r.mu.RUnlock()
	if node == nil || now.Sub(last) >= 5*time.Second {
		n, err := r.Store.Authenticate(ctx, r.cfg.Cluster.NodeID, r.certificate)
		r.mu.Lock()
		if err == nil {
			r.local = n
			r.lastConfig = now
			node = n
			last = now
		} else {
			// Authentication refusal is also a reason to leave the pool, and it used to be as
			// silent as drift: the registry had forgotten this node, was still pending, or the
			// control database was briefly unavailable. Record and log it once per change.
			reason := "registration not recognised: " + err.Error()
			if r.lastMismatch != reason {
				r.lastMismatch = reason
				logger.LegacyPrintf("cluster.runtime", "cluster admission withheld on %s: %s", r.cfg.Cluster.NodeID, reason)
			}
			r.mismatch = reason
			r.local = nil
			node = nil
		}
		r.mu.Unlock()
	}
	if node != nil && registrationMismatch(*node, r.cfg.Cluster.Roles, r.cfg.Cluster.Capabilities) != "" {
		// Roles and capabilities define whether this node is safe to receive this cluster's work.
		// Version remains observable in snapshots but does not block mixed-version rolling upgrades.
		mismatch := registrationMismatch(*node, r.cfg.Cluster.Roles, r.cfg.Cluster.Capabilities)
		if r.lastMismatch != mismatch {
			r.lastMismatch = mismatch
			logger.LegacyPrintf("cluster.runtime", "cluster admission withheld on %s: %s", r.cfg.Cluster.NodeID, mismatch)
		}
		r.mismatch = mismatch
		node = nil
	} else if node != nil {
		r.mismatch = ""
	}
	dbOK := r.db.PingContext(ctx) == nil
	redisOK := false
	if r.redis != nil {
		info, err := r.redis.Info(ctx, "server").Result()
		if err == nil {
			for _, line := range strings.Split(info, "\n") {
				if strings.HasPrefix(line, "run_id:") {
					id := strings.TrimSpace(strings.TrimPrefix(line, "run_id:"))
					allowed, checkErr := r.Store.CheckRedisGeneration(ctx, id)
					redisOK = checkErr == nil && allowed
					if redisOK {
						redisOK = r.redis.Set(ctx, "cluster:v3:"+r.cfg.Cluster.ID+":admission-generation", id, 5*time.Second).Err() == nil
					}
					break
				}
			}
		}
	}
	var billingMode string
	if r.db.QueryRowContext(ctx, `SELECT settlement_mode FROM cluster_control WHERE cluster_id=$1`, r.cfg.Cluster.ID).Scan(&billingMode) != nil || billingMode != "atomic" {
		dbOK = false
	}
	if !r.recoveryHealthy.Load() {
		dbOK = false
	}
	a, w, p := r.Gate.Counts()
	ready := !r.stopping.Load() && node != nil && node.State == "ready" && dbOK && redisOK && p < max(128, r.cfg.Cluster.MaxInFlight*4) && now.Sub(last) < 15*time.Second
	if node != nil {
		r.Gate.Configure(ready, node.MaxInFlight, node.MaxQueue)
		r.db.SetMaxOpenConns(node.DBConnections)
		r.db.SetMaxIdleConns(min(r.cfg.Database.MaxIdleConns, node.DBConnections))
	} else {
		r.Gate.Configure(false, r.cfg.Cluster.MaxInFlight, r.cfg.Cluster.MaxQueue)
	}
	snapshot := Snapshot{NodeID: r.cfg.Cluster.NodeID, BootID: r.bootID, At: now, Active: a, Waiting: w, PendingSettlement: p, DBOK: dbOK, RedisOK: redisOK, AdmissionReady: ready, DBConnections: r.db.Stats().InUse, QueueWaitMS: r.Gate.QueueWaitMS(), AdmissionBlockedBy: r.mismatch}
	if node != nil {
		snapshot.ObservedRevision = node.Revision
		snapshot.Version = r.cfg.Cluster.ApplicationVersion
	}
	dbStats := r.db.Stats()
	snapshot.DBPoolMax = &dbStats.MaxOpenConnections
	snapshot.DBPoolIdle = &dbStats.Idle
	if r.redis != nil {
		pool := r.redis.PoolStats()
		limit, total, idle := r.redis.Options().PoolSize, int(pool.TotalConns), int(pool.IdleConns)
		snapshot.RedisPoolMax, snapshot.RedisPoolTotal, snapshot.RedisPoolIdle = &limit, &total, &idle
	}
	if counters, err := gnet.IOCountersWithContext(ctx, true); err == nil {
		var total int64
		for _, c := range counters {
			if c.Name != "lo" {
				total += int64(c.BytesSent + c.BytesRecv)
			}
		}
		snapshot.RawTrafficBytes = &total
	}
	if node != nil && r.trafficBootID != "" && node.TrafficSource == "interface_estimate" && snapshot.RawTrafficBytes != nil && (r.lastTrafficCheckpoint.IsZero() || now.Sub(r.lastTrafficCheckpoint) >= time.Minute) {
		if usage, err := r.Store.ObserveTraffic(ctx, *node, r.trafficBootID, *snapshot.RawTrafficBytes, now); err == nil {
			snapshot.CycleTrafficBytes = &usage
			r.lastTrafficCheckpoint = now
		}
	} else if node != nil {
		if previous, err := r.Snapshot(ctx, node.ID); err == nil && previous != nil {
			snapshot.CycleTrafficBytes = previous.CycleTrafficBytes
		}
	}
	if node != nil && TrafficBudgetExhausted(*node, snapshot.CycleTrafficBytes) {
		r.Gate.Configure(false, node.MaxInFlight, node.MaxQueue)
		snapshot.AdmissionReady = false
		// The gate is closed by a budget, not by a fault; say so instead of leaving the
		// console to display the previous, now unrelated reason.
		snapshot.AdmissionBlockedBy = "monthly traffic budget exhausted"
		r.mismatch = snapshot.AdmissionBlockedBy
	}
	if r.redis != nil {
		data, err := json.Marshal(snapshot)
		if err == nil {
			_ = r.redis.Set(ctx, r.snapshotKey(r.cfg.Cluster.NodeID), data, 20*time.Second).Err()
		}
	}
}
func (r *Runtime) peer(req *http.Request) (string, string, error) {
	if req.TLS == nil || len(req.TLS.VerifiedChains) == 0 || len(req.TLS.PeerCertificates) == 0 {
		return "", "", ErrForbidden
	}
	cert := req.TLS.PeerCertificates[0]
	h := sha256.Sum256(cert.Raw)
	return cert.Subject.CommonName, hex.EncodeToString(h[:]), nil
}
func (r *Runtime) health(w http.ResponseWriter, req *http.Request) {
	id, cert, err := r.peer(req)
	if err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	node, err := r.Store.Authenticate(req.Context(), id, cert)
	if err != nil || !slices.Contains(node.Roles, "controller") {
		http.Error(w, "forbidden", 403)
		return
	}
	a, q, p := r.Gate.Counts()
	ready := r.Gate.Ready()
	dependenciesHealthy := ready
	if !dependenciesHealthy {
		snapshot, snapshotErr := r.Snapshot(req.Context(), r.cfg.Cluster.NodeID)
		dependenciesHealthy = snapshotErr == nil && snapshot != nil && probeDependenciesHealthy(*snapshot, time.Now())
	}
	if !dependenciesHealthy {
		http.Error(w, "dependencies not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"node_id": r.NodeID(), "boot_id": r.bootID, "ready": ready, "active": a, "waiting": q, "pending_settlement": p})
}

func probeDependenciesHealthy(snapshot Snapshot, now time.Time) bool {
	return snapshot.Fresh(now) && snapshot.DBOK && snapshot.RedisOK && snapshot.PendingSettlement == 0
}
func (r *Runtime) enroll(w http.ResponseWriter, req *http.Request) {
	if !r.cfg.RunsRole(config.RoleController) {
		http.NotFound(w, req)
		return
	}
	id, cert, err := r.peer(req)
	if err != nil {
		http.Error(w, "forbidden", 403)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	n, err := r.Store.Enroll(req.Context(), id, cert, body.Token)
	if err != nil {
		http.Error(w, "enrollment rejected", 403)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(n)
}

// IsGatewayPath includes streaming GET aliases; the HTTP method alone is not an execution boundary.
func IsGatewayPath(path string) bool {
	for _, prefix := range []string{"/v1/", "/v1beta/", "/responses", "/chat/completions", "/embeddings", "/images/", "/videos", "/tts", "/stt", "/custom-voices", "/web_search", "/x_search", "/realtime", "/backend-api/codex/", "/antigravity/", "/alpha/search", "/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks", "/contents/generations/tasks"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func ValidPeerAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && net.ParseIP(host) != nil
}

// Recovery has its own budget; slow durable replay must not starve two-second telemetry.
func (r *Runtime) recoveryLoop(parent context.Context) {
	defer close(r.recoveryDone)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		healthy := false
		pending := 1
		if r.recoverSettlements != nil {
			var err error
			pending, err = r.recoverSettlements(ctx)
			healthy = err == nil
		}
		if healthy {
			unresolved, err := r.Store.Unsettled(ctx, r.cfg.Cluster.NodeID)
			healthy = err == nil
			pending = max(pending, unresolved)
		}
		r.Gate.SetPending(max(0, pending))
		r.recoveryHealthy.Store(healthy)
		cancel()
		select {
		case <-parent.Done():
			return
		case <-ticker.C:
		}
	}
}
