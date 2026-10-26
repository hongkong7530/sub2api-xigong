package cluster

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestIngressProxyConfirmationRequiresEveryServingProcess(t *testing.T) {
	now := time.Now().UTC()
	worker := fixtureNode()
	a, b := fixtureNode(), fixtureNode()
	a.ID, a.Roles = "ingress-a", []string{"ingress", "worker", "controller"}
	b.ID, b.Roles = "ingress-b", []string{"ingress", "worker"}
	nodes := []Node{worker, a, b}
	revision := proxyRevision(nodes)
	makeSnapshots := func() map[string]*Snapshot {
		result := make(map[string]*Snapshot)
		for _, n := range []Node{a, b} {
			result[n.ID] = &Snapshot{NodeID: n.ID, BootID: n.ID + "-boot", At: now, ObservedRevision: n.Revision,
				Proxy: &ProxyObservation{BootID: n.ID + "-boot", At: now, Revision: revision}}
		}
		return result
	}
	require.True(t, ingressProxiesApplied(nodes, makeSnapshots(), revision, now))
	for name, mutate := range map[string]func(map[string]*Snapshot){
		"missing":          func(s map[string]*Snapshot) { delete(s, b.ID) },
		"old node":         func(s map[string]*Snapshot) { s[b.ID].Proxy = nil },
		"stale heartbeat":  func(s map[string]*Snapshot) { s[b.ID].At = now.Add(-time.Minute) },
		"stale proxy":      func(s map[string]*Snapshot) { s[b.ID].Proxy.At = now.Add(-time.Minute) },
		"future proxy":     func(s map[string]*Snapshot) { s[b.ID].Proxy.At = now.Add(time.Minute) },
		"restarted":        func(s map[string]*Snapshot) { s[b.ID].BootID = "new-boot" },
		"wrong plan":       func(s map[string]*Snapshot) { s[b.ID].Proxy.Revision = "old-plan" },
		"reload failed":    func(s map[string]*Snapshot) { s[b.ID].Proxy.Error = "proxy_configuration_failed" },
		"wrong identity":   func(s map[string]*Snapshot) { s[b.ID].NodeID = a.ID },
		"old registration": func(s map[string]*Snapshot) { s[b.ID].ObservedRevision-- },
	} {
		t.Run(name, func(t *testing.T) {
			snapshots := makeSnapshots()
			mutate(snapshots)
			require.False(t, ingressProxiesApplied(nodes, snapshots, revision, now))
		})
	}
	for _, state := range []string{"retired", "pending"} {
		changed := append([]Node(nil), nodes...)
		changed[2].State = state
		snapshots := makeSnapshots()
		delete(snapshots, b.ID)
		require.True(t, ingressProxiesApplied(changed, snapshots, revision, now), state)
	}
	require.False(t, ingressProxiesApplied([]Node{worker}, nil, revision, now))
}

func TestProxyPlanStableAcrossAcknowledgementsButNotCapacityRevisions(t *testing.T) {
	a, b := fixtureNode(), fixtureNode()
	b.ID = "node-2"
	nodes := []Node{a, b}
	want := proxyRevision(nodes)
	nodes[0].AppliedRevision = nodes[0].Revision
	nodes[0].ApplyError = "old error cleared later"
	require.Equal(t, want, proxyRevision(nodes))
	require.Equal(t, want, proxyRevision([]Node{nodes[1], nodes[0]}))
	nodes[0].Revision++
	nodes[0].MaxInFlight *= 2
	require.NotEqual(t, want, proxyRevision(nodes))
}

func TestProxyLockReleaseEvictsUncertainSession(t *testing.T) {
	for _, failure := range []string{"none", "error", "not owner"} {
		t.Run(failure, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			conn, err := db.Conn(context.Background())
			require.NoError(t, err)
			query := mock.ExpectQuery("SELECT pg_advisory_unlock").WithArgs("test:ingress")
			switch failure {
			case "error":
				query.WillReturnError(errors.New("unlock interrupted"))
			case "not owner":
				query.WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(false))
			default:
				query.WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
			}
			releaseProxyLock(conn, "test:ingress")
			if failure == "none" {
				require.NoError(t, conn.Close())
				require.Equal(t, 1, db.Stats().OpenConnections)
			} else {
				require.ErrorIs(t, conn.PingContext(context.Background()), sql.ErrConnDone)
				require.Zero(t, db.Stats().OpenConnections, "an uncertain session lock must never return to the pool")
			}
		})
	}
}

func capacityTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var name string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name))
	require.Contains(t, name, "cluster_acceptance")
	return db, dsn
}

func TestProxyLockReleasePostgres(t *testing.T) {
	db, dsn := capacityTestDatabase(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	observer, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer observer.Close()
	observer.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, mode := range []string{"normal", "no longer owned", "session terminated"} {
		t.Run(mode, func(t *testing.T) {
			conn, err := db.Conn(ctx)
			require.NoError(t, err)
			defer conn.Close()
			key := fmt.Sprintf("capacity-lock-%d", time.Now().UnixNano())
			var pid int
			var locked bool
			require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_backend_pid(), pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&pid, &locked))
			require.True(t, locked)
			if mode == "no longer owned" {
				require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key).Scan(&locked))
				require.True(t, locked)
			} else if mode == "session terminated" {
				require.NoError(t, observer.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&locked))
				require.True(t, locked)
			}
			releaseProxyLock(conn, key)
			_ = conn.Close()
			var locks, nextPID int
			require.NoError(t, observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory'", pid).Scan(&locks))
			require.Zero(t, locks)
			require.NoError(t, db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&nextPID))
			if mode == "normal" {
				require.Equal(t, pid, nextPID)
			} else {
				require.NotEqual(t, pid, nextPID, "uncertain sessions must be physically evicted")
			}
		})
	}
}

func TestPoolMultiplexes2500InFlightRequestsPostgres(t *testing.T) {
	_, dsn := capacityTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	const requests = 2500
	type node struct {
		id                   string
		gate, pool, requests int
	}
	layout := []node{{"small", 280, 64, 200}, {"medium", 1200, 88, 900}, {"large", 2000, 96, 1400}}
	var reached, completed atomic.Int64
	var workers sync.WaitGroup
	failures := make(chan error, requests)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	pools := make([]*sql.DB, 0, len(layout))
	started := time.Now()
	for _, n := range layout {
		db, err := sql.Open("postgres", dsn)
		require.NoError(t, err)
		defer db.Close()
		db.SetMaxOpenConns(n.pool)
		db.SetMaxIdleConns(16)
		pools = append(pools, db)
		gate := NewGate(n.gate, 64)
		gate.Configure(true, n.gate, 64)
		for i := 0; i < n.requests; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				done, err := gate.Acquire(ctx, time.Second)
				if err != nil {
					failures <- err
					return
				}
				defer done()
				var value int
				if err = db.QueryRowContext(ctx, "SELECT 1").Scan(&value); err != nil {
					failures <- err
					return
				}
				reached.Add(1)
				// Upstream waiting holds a request slot, never a database transaction or connection.
				select {
				case <-release:
				case <-ctx.Done():
					failures <- ctx.Err()
					return
				}
				if err = db.QueryRowContext(ctx, "SELECT 1").Scan(&value); err != nil {
					failures <- err
					return
				}
				completed.Add(1)
			}()
		}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for reached.Load() != requests {
		select {
		case err := <-failures:
			unblock()
			workers.Wait()
			t.Fatalf("isolated request failed: %v", err)
		case <-ctx.Done():
			unblock()
			workers.Wait()
			t.Fatalf("isolated hold barrier: %v; reached=%d", ctx.Err(), reached.Load())
		case <-ticker.C:
		}
	}
	open, busy := 0, 0
	for _, pool := range pools {
		stats := pool.Stats()
		open += stats.OpenConnections
		busy += stats.InUse
	}
	require.LessOrEqual(t, open, 248)
	require.Zero(t, busy, "all 2500 upstream waits must release database connections")
	holdElapsed := time.Since(started)
	unblock()
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Equal(t, int64(requests), completed.Load())
	var waitCount int64
	for _, pool := range pools {
		waitCount += pool.Stats().WaitCount
	}
	t.Logf("isolated lifecycle requests=%d completed=%d pool_cap=248 held_open=%d held_busy=%d reach_ms=%d total_ms=%d pool_wait_count=%d; SELECT 1 only, no full gateway, WAN or model generation", requests, completed.Load(), open, busy, holdElapsed.Milliseconds(), time.Since(started).Milliseconds(), waitCount)
}

func TestProxyObservationRoundTripDoesNotInventLegacyConfirmation(t *testing.T) {
	var old Snapshot
	require.NoError(t, json.Unmarshal([]byte(`{"node_id":"old","boot_id":"old-boot"}`), &old))
	require.Nil(t, old.Proxy)
	current := Snapshot{NodeID: "new", BootID: "new-boot", Proxy: &ProxyObservation{BootID: "new-boot", At: time.Now().UTC(), Revision: "plan"}}
	data, err := json.Marshal(current)
	require.NoError(t, err)
	var decoded Snapshot
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, current.Proxy, decoded.Proxy)
}
