package cluster

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func fixtureNode() Node {
	return Node{ClusterID: "test", ID: "node-1", Name: "Node 1", Roles: []string{"worker"}, Capabilities: []string{"http"}, Address: "127.0.0.1:19180", CertificateSHA256: strings.Repeat("a", 64), State: "ready", Revision: 1, BaseWeight: 8, MaxInFlight: 4, MaxQueue: 2, DBConnections: 8, TrafficCycleDay: 1, TrafficTimezone: "UTC", TrafficSource: "unknown"}
}

func TestGateDrainWakesQueuedRequestsAndKeepsActiveWork(t *testing.T) {
	g := NewGate(1, 1)
	g.Configure(true, 1, 1)
	release, err := g.Acquire(context.Background(), time.Second)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, err := g.Acquire(context.Background(), time.Second); done <- err }()
	require.Eventually(t, func() bool { _, w, _ := g.Counts(); return w == 1 }, time.Second, time.Millisecond)
	g.Configure(false, 1, 1)
	require.ErrorIs(t, <-done, ErrNotReady)
	a, w, _ := g.Counts()
	require.Equal(t, 1, a)
	require.Zero(t, w)
	release()
	release()
	a, _, _ = g.Counts()
	require.Zero(t, a)
}
func TestGateConcurrentCapacityAndTimeout(t *testing.T) {
	g := NewGate(2, 0)
	g.Configure(true, 2, 0)
	var releases []func()
	for i := 0; i < 2; i++ {
		release, err := g.Acquire(context.Background(), time.Second)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, err := g.Acquire(context.Background(), time.Second)
	require.ErrorIs(t, err, ErrBusy)
	var wg sync.WaitGroup
	for _, release := range releases {
		wg.Add(1)
		go func(f func()) { defer wg.Done(); f() }(release)
	}
	wg.Wait()
	g.SetPending(1)
	_, _, pending := g.Counts()
	require.Equal(t, 1, pending)
	g.Configure(false, 2, 0)
	_, err = g.Acquire(context.Background(), time.Second)
	require.ErrorIs(t, err, ErrNotReady)
}
func TestRegistrationMismatchAllowsMixedVersionsButRequiresSameRolesAndCapabilities(t *testing.T) {
	node := fixtureNode()
	node.Version = "0.2.15-custom-cluster-v3"

	require.Empty(t, registrationMismatch(node, node.Roles, node.Capabilities), "mixed-version rolling upgrades must not be blocked")
	require.Equal(t, "role mismatch", registrationMismatch(node, []string{"ingress", "worker"}, node.Capabilities))
	require.Equal(t, "capability mismatch", registrationMismatch(node, node.Roles, []string{"http", "images"}))
}

func TestNodeValidation(t *testing.T) {
	n := fixtureNode()
	require.NoError(t, n.Validate())
	n.Address = "example.com:443"
	require.Error(t, n.Validate())
	n = fixtureNode()
	n.CertificateSHA256 = ""
	require.Error(t, n.Validate())
}
func TestProxyManagedRenderingAndPreservation(t *testing.T) {
	p := NewProxy(config.ClusterConfig{CAFile: "/etc/cluster/ca.pem", StateDir: "/var/lib/cluster"}, "/run/haproxy/admin.sock", "workers")
	original := "global\n    maxconn 1000\n" + managedBegin + "\nold\n" + managedEnd + "\nbackend database\n    mode tcp\n"
	out, err := p.Render(original, []Node{fixtureNode()})
	require.NoError(t, err)
	require.Contains(t, out, "backend database\n    mode tcp")
	require.Contains(t, out, "verify required verifyhost node-1")
	require.Contains(t, out, "weight 8 maxconn 4")
	n := fixtureNode()
	n.State = "paused"
	out, err = p.Render(original, []Node{n})
	require.NoError(t, err)
	require.Contains(t, out, " disabled")
	n.State = "retired"
	out, err = p.Render(original, []Node{n})
	require.NoError(t, err)
	require.NotContains(t, out, "server node-1")
	_, err = p.Render("backend unrelated", nil)
	require.Error(t, err)
}
func TestDesiredWeightDoesNotTreatUnknownAsZero(t *testing.T) {
	n := fixtureNode()
	now := time.Now()
	require.Equal(t, 8, DesiredWeight(n, nil, now))
	s := &Snapshot{At: now.Add(-time.Minute)}
	require.Equal(t, 8, DesiredWeight(n, s, now))
	s.At = now
	require.Zero(t, DesiredWeight(n, s, now))
	s.DBOK = true
	s.RedisOK = true
	s.Active = 4
	require.Equal(t, 4, DesiredWeight(n, s, now))
	limit, used := int64(100), int64(95)
	n.MonthlyLimitBytes = &limit
	n.TrafficReserveBytes = 10
	s.CycleTrafficBytes = &used
	require.Zero(t, DesiredWeight(n, s, now))
}
func TestGatewayAliasesIncludeStreamingAndTasks(t *testing.T) {
	for _, path := range []string{"/responses", "/realtime", "/v1/live/call", "/alpha/search", "/api/v3/contents/generations/tasks", "/contents/generations/tasks/a"} {
		require.True(t, IsGatewayPath(path), path)
	}
	require.False(t, IsGatewayPath("/api/v1/admin/ops"))
	require.False(t, errors.Is(ErrBusy, ErrNotReady))
}
