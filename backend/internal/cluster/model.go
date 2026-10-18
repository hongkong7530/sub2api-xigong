// Package cluster owns v3 membership and node admission independently of gateway billing.
package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"
)

var (
	ErrInvalid    = errors.New("invalid cluster operation")
	ErrConflict   = errors.New("cluster revision or operation conflict")
	ErrNotFound   = errors.New("cluster node not found")
	ErrNotReady   = errors.New("cluster node is not ready for new work")
	ErrBusy       = errors.New("cluster node admission budget is exhausted")
	ErrUnsafeExit = errors.New("node still has in-flight, queued or unsettled work")
	ErrForbidden  = errors.New("cluster identity is not authorized")
)

var identityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Node struct {
	ClusterID           string   `json:"cluster_id"`
	ID                  string   `json:"node_id"`
	Name                string   `json:"name"`
	Roles               []string `json:"roles"`
	Capabilities        []string `json:"capabilities"`
	Address             string   `json:"address"`
	CertificateSHA256   string   `json:"certificate_sha256"`
	State               string   `json:"state"`
	Revision            int64    `json:"revision"`
	AppliedRevision     int64    `json:"applied_revision"`
	ApplyError          string   `json:"apply_error,omitempty"`
	BaseWeight          int      `json:"base_weight"`
	MaxInFlight         int      `json:"max_in_flight"`
	MaxQueue            int      `json:"max_queue"`
	DBConnections       int      `json:"db_connections"`
	MonthlyLimitBytes   *int64   `json:"monthly_limit_bytes"`
	TrafficReserveBytes int64    `json:"traffic_reserve_bytes"`
	TrafficCycleDay     int      `json:"traffic_cycle_day"`
	TrafficTimezone     string   `json:"traffic_timezone"`
	TrafficSource       string   `json:"traffic_source"`
	Version             string   `json:"version"`
}

func (n Node) Validate() error {
	if !identityPattern.MatchString(n.ClusterID) || !identityPattern.MatchString(n.ID) || n.Name == "" || len(n.Name) > 128 {
		return fmt.Errorf("invalid cluster/node identity")
	}
	host, port, splitErr := net.SplitHostPort(n.Address)
	if splitErr == nil {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 || net.ParseIP(host) == nil {
			return fmt.Errorf("invalid node IP or port")
		}
	}
	if _, _, err := net.SplitHostPort(n.Address); err != nil {
		return fmt.Errorf("invalid node address: %w", err)
	}
	if ip, _, _ := net.SplitHostPort(n.Address); net.ParseIP(ip) == nil {
		return fmt.Errorf("node address must use a verified IP, not a rebindable hostname")
	}
	if len(n.CertificateSHA256) != 64 {
		return fmt.Errorf("node certificate fingerprint is required")
	}
	if _, err := hex.DecodeString(n.CertificateSHA256); err != nil {
		return fmt.Errorf("invalid certificate fingerprint")
	}
	if n.BaseWeight < 1 || n.BaseWeight > 256 || n.MaxInFlight < 1 || n.MaxInFlight > 100000 || n.MaxQueue < 0 || n.MaxQueue > 100000 || n.DBConnections < 1 {
		return fmt.Errorf("invalid node capacity")
	}
	if len(n.Roles) == 0 {
		return fmt.Errorf("node roles are required")
	}
	seen := map[string]bool{}
	for _, role := range n.Roles {
		if seen[role] || !slices.Contains([]string{"worker", "ingress", "controller", "maintenance"}, role) {
			return fmt.Errorf("invalid node role")
		}
		seen[role] = true
	}
	for _, capability := range n.Capabilities {
		if !slices.Contains([]string{"http", "websocket", "live", "plugins", "image_tasks", "batch_image"}, capability) {
			return fmt.Errorf("invalid node capability")
		}
	}
	if slices.Contains(n.Roles, "maintenance") && !slices.Contains(n.Roles, "worker") {
		return fmt.Errorf("maintenance requires local worker capability")
	}
	if n.MonthlyLimitBytes != nil && (*n.MonthlyLimitBytes < 0 || n.TrafficReserveBytes > *n.MonthlyLimitBytes) {
		return fmt.Errorf("invalid monthly traffic budget")
	}
	if n.TrafficReserveBytes < 0 || n.TrafficCycleDay < 1 || n.TrafficCycleDay > 28 {
		return fmt.Errorf("invalid traffic cycle")
	}
	if _, err := time.LoadLocation(n.TrafficTimezone); err != nil {
		return fmt.Errorf("invalid traffic timezone")
	}
	if !slices.Contains([]string{"unknown", "interface_estimate"}, n.TrafficSource) {
		return fmt.Errorf("invalid traffic source")
	}
	return nil
}

type Snapshot struct {
	NodeID            string    `json:"node_id"`
	BootID            string    `json:"boot_id"`
	At                time.Time `json:"sampled_at"`
	Active            int       `json:"active"`
	Waiting           int       `json:"waiting"`
	PendingSettlement int       `json:"pending_settlement"`
	QueueWaitMS       float64   `json:"queue_wait_ms"`
	DBOK              bool      `json:"db_ok"`
	RedisOK           bool      `json:"redis_ok"`
	AdmissionReady    bool      `json:"admission_ready"`
	ObservedRevision  int64     `json:"observed_revision"`
	RawTrafficBytes   *int64    `json:"raw_traffic_bytes"`
	CycleTrafficBytes *int64    `json:"cycle_traffic_bytes"`
	DBConnections     int       `json:"db_connections"`
	DBPoolMax         *int      `json:"db_pool_max,omitempty"`
	DBPoolIdle        *int      `json:"db_pool_idle,omitempty"`
	RedisPoolMax      *int      `json:"redis_pool_max,omitempty"`
	RedisPoolTotal    *int      `json:"redis_pool_total,omitempty"`
	RedisPoolIdle     *int      `json:"redis_pool_idle,omitempty"`
	Version           string    `json:"version"`
	// AdmissionBlockedBy names a role/capability self-check that withholds admission.
	AdmissionBlockedBy string `json:"admission_blocked_by,omitempty"`
}

func (s Snapshot) Fresh(now time.Time) bool {
	return !s.At.IsZero() && !s.At.After(now.Add(5*time.Second)) && now.Sub(s.At) <= 15*time.Second
}

type Operation struct {
	ID             string    `json:"id"`
	NodeID         string    `json:"node_id"`
	RequestKey     string    `json:"request_key"`
	Action         string    `json:"action"`
	TargetRevision int64     `json:"target_revision"`
	Status         string    `json:"status"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Change struct {
	RequestKey       string `json:"request_key"`
	ExpectedRevision int64  `json:"expected_revision"`
	Action           string `json:"action"`
	ConfirmForce     bool   `json:"confirm_force"`
	Node             *Node  `json:"node,omitempty"`
}

func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Gate counts logical work, not TCP connections. Retiring prevents queued work from starting.
type Gate struct {
	mu          sync.Mutex
	ready       bool
	active      int
	waiting     int
	pending     int
	maxActive   int
	maxWaiting  int
	changed     chan struct{}
	waitSamples int64
	waitMS      float64
}

func NewGate(active, waiting int) *Gate {
	return &Gate{maxActive: active, maxWaiting: waiting, changed: make(chan struct{})}
}
func (g *Gate) signal() { close(g.changed); g.changed = make(chan struct{}) }
func (g *Gate) Configure(ready bool, active, waiting int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ready = ready
	g.maxActive = active
	g.maxWaiting = waiting
	g.signal()
}
func (g *Gate) Ready() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.ready }
func (g *Gate) Counts() (int, int, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active, g.waiting, g.pending
}
func (g *Gate) SetPending(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n == g.pending {
		return
	}
	g.pending = n
	g.signal()
}

func (g *Gate) Acquire(ctx context.Context, timeout time.Duration) (func(), error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	g.mu.Lock()
	if !g.ready {
		g.mu.Unlock()
		return nil, ErrNotReady
	}
	queued := false
	for g.active >= g.maxActive {
		if !queued {
			if g.waiting >= g.maxWaiting {
				g.mu.Unlock()
				return nil, ErrBusy
			}
			g.waiting++
			queued = true
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.mu.Lock()
			g.waiting--
			g.mu.Unlock()
			return nil, ErrBusy
		case <-changed:
		}
		g.mu.Lock()
		if !g.ready {
			if queued {
				g.waiting--
			}
			g.mu.Unlock()
			return nil, ErrNotReady
		}
	}
	if queued {
		g.waiting--
	}
	if err := ctx.Err(); err != nil {
		g.mu.Unlock()
		return nil, err
	}
	waitMS := float64(time.Since(started).Microseconds()) / 1000
	if g.waitSamples == 0 {
		g.waitMS = waitMS
	} else {
		g.waitMS = .8*g.waitMS + .2*waitMS
	}
	g.waitSamples++
	g.active++
	g.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.active--; g.signal(); g.mu.Unlock() }) }, nil
}

func registrationMismatch(node Node, roles, capabilities []string) string {
	if !sameStringSet(node.Roles, roles) {
		return "role mismatch"
	}
	if !sameStringSet(node.Capabilities, capabilities) {
		return "capability mismatch"
	}
	return ""
}

func sameStringSet(a, b []string) bool {
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func (g *Gate) QueueWaitMS() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.waitSamples == 0 {
		return 0
	}
	return g.waitMS
}
