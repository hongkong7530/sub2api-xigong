package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestProbeDependenciesHealthyIsIndependentOfAdmission(t *testing.T) {
	now := time.Now()
	fresh := Snapshot{At: now, DBOK: true, RedisOK: true, PendingSettlement: 0, AdmissionReady: false}
	require.True(t, probeDependenciesHealthy(fresh, now), "pending nodes must be probeable before activation")
	fresh.DBOK = false
	require.False(t, probeDependenciesHealthy(fresh, now))
	fresh.DBOK = true
	fresh.RedisOK = false
	require.False(t, probeDependenciesHealthy(fresh, now))
	fresh.RedisOK = true
	fresh.PendingSettlement = 1
	require.False(t, probeDependenciesHealthy(fresh, now))
	fresh.PendingSettlement = 0
	fresh.At = now.Add(-time.Minute)
	require.False(t, probeDependenciesHealthy(fresh, now))
}

func TestIngressReconcileUsesLocalLockWithoutAuthorityWrites(t *testing.T) {
	for _, tc := range []struct {
		id                                                             string
		controller, staleCapacity, ignoreCapacity, otherIngressMissing bool
	}{
		{id: "ingress-a"},
		{id: "ingress-b", staleCapacity: true},
		{id: "ingress-capacity-stale", staleCapacity: true, ignoreCapacity: true},
		{id: "controller", controller: true, staleCapacity: true},
		{id: "controller-waiting", controller: true, otherIngressMissing: true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			// Keep Unix socket paths short enough for both Linux and Windows.
			dir, err := os.MkdirTemp("", "cluster-ingress-")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			socket := filepath.Join(dir, "proxy.sock")
			listener, err := net.Listen("unix", socket)
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			var mu sync.Mutex
			var commands []string
			node := fixtureNode()
			node.ID, node.Roles = tc.id, []string{config.RoleIngress, config.RoleWorker}
			if tc.controller {
				node.Roles = append(node.Roles, config.RoleController)
			}
			capacity := node.MaxInFlight
			if tc.staleCapacity {
				capacity = 2
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					_ = conn.SetDeadline(time.Now().Add(time.Second))
					line, _ := bufio.NewReader(conn).ReadString('\n')
					command := strings.TrimSpace(line)
					mu.Lock()
					commands = append(commands, command)
					mu.Unlock()
					if strings.HasPrefix(command, "set maxconn server ") && strings.HasSuffix(command, " 4") && !tc.ignoreCapacity {
						capacity = 4
					}
					if command == "show stat" {
						_, _ = fmt.Fprintf(conn, "workers,%s,0,0,0,0,%d,0,0,0,0,0,0,0,0,0,0,UP,8\n", node.ID, capacity)
					}
					_ = conn.Close()
				}
			}()
			t.Cleanup(func() { _ = listener.Close(); <-done })
			cfg := &config.Config{Cluster: config.ClusterConfig{Enabled: true, ID: "test", NodeID: tc.id, Roles: node.Roles, HAProxyConfig: filepath.Join(dir, "haproxy.cfg"), CAFile: "/etc/cluster/ca.pem", StateDir: "/var/lib/cluster"}}
			proxy := NewProxy(cfg.Cluster, socket, "workers")
			retired := node
			retired.ID, retired.State = "retired-node", "retired"
			nodes := []Node{node, retired}
			if tc.otherIngressMissing {
				other := fixtureNode()
				other.ID, other.Roles = "missing-ingress", []string{config.RoleIngress}
				nodes = append(nodes, other)
			}
			rendered, err := proxy.Render(managedBegin+"\n"+managedEnd, nodes)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfg.Cluster.HAProxyConfig, []byte(rendered), 0600))
			rows := sqlmock.NewRows([]string{"cluster_id", "node_id", "name", "roles", "capabilities", "address", "certificate_sha256", "state", "revision", "base_weight", "max_in_flight", "max_queue", "db_connections", "monthly_limit_bytes", "traffic_reserve_bytes", "traffic_cycle_day", "traffic_timezone", "traffic_source", "version", "applied_revision", "apply_error"})
			for _, n := range nodes {
				roles, _ := json.Marshal(n.Roles)
				capabilities, _ := json.Marshal(n.Capabilities)
				rows.AddRow(n.ClusterID, n.ID, n.Name, roles, capabilities, n.Address, n.CertificateSHA256, n.State, n.Revision, n.BaseWeight, n.MaxInFlight, n.MaxQueue, n.DBConnections, nil, n.TrafficReserveBytes, n.TrafficCycleDay, n.TrafficTimezone, n.TrafficSource, n.Version, n.AppliedRevision, n.ApplyError)
			}
			mock.ExpectQuery("SELECT .* FROM cluster_nodes").WithArgs("test").WillReturnRows(rows)
			lock := "test:ingress"
			if !tc.controller {
				lock += ":" + tc.id
			}
			mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(lock).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			if tc.controller && !tc.otherIngressMissing {
				mock.ExpectExec("UPDATE cluster_nodes SET applied_revision").WithArgs("test", node.ID, node.Revision).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE cluster_operations SET status='applied'").WithArgs("test", node.ID, node.Revision).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE cluster_nodes SET applied_revision").WithArgs("test", retired.ID, retired.Revision).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE cluster_operations SET status='awaiting_revocation'").WithArgs("test", retired.ID, retired.Revision).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectQuery("SELECT pg_advisory_unlock").WithArgs(lock).WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			r := &Runtime{cfg: cfg, db: db, redis: client, proxy: proxy, bootID: "current-boot", Store: &Store{DB: db, ClusterID: "test"}}
			snapshot, err := json.Marshal(Snapshot{NodeID: node.ID, BootID: r.bootID, At: time.Now(), DBOK: true, RedisOK: true, AdmissionReady: true, ObservedRevision: node.Revision, Proxy: &ProxyObservation{BootID: r.bootID, At: time.Now(), Revision: proxyRevision(nodes)}})
			require.NoError(t, err)
			server.Set(r.snapshotKey(node.ID), string(snapshot))
			r.reconcile(context.Background())
			mu.Lock()
			got := append([]string(nil), commands...)
			mu.Unlock()
			want := []string{"show stat"}
			if tc.staleCapacity {
				want = append(want, "set maxconn server workers/"+node.ID+" 4")
			}
			want = append(want, "set server workers/"+node.ID+" weight 8", "set server workers/"+node.ID+" state ready", "show stat")
			require.Equal(t, want, got)
			require.Equal(t, tc.controller, r.IsController())
			if tc.ignoreCapacity {
				require.Equal(t, "proxy_runtime_unconfirmed", r.proxyObservation.Error)
			} else {
				require.Empty(t, r.proxyObservation.Error)
				require.Equal(t, proxyRevision(nodes), r.proxyObservation.Revision)
			}
			require.NoError(t, mock.ExpectationsWereMet(), "a single connection must cover lock and authority writes")
			if !tc.controller {
				r.markApplyError(context.Background(), nil, "proxy_readback_failed")
				require.Equal(t, "proxy_readback_failed", r.lastProxyError)
				require.Equal(t, "proxy_readback_failed", r.proxyObservation.Error)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestShadowIngressDoesNotReconcileOrAcquireDatabaseConnection(t *testing.T) {
	r := &Runtime{cfg: &config.Config{Cluster: config.ClusterConfig{Enabled: true, Shadow: true, Roles: []string{config.RoleIngress}}}, proxy: &Proxy{}}
	// A shadow runtime has no live DB/proxy dependencies and must return before touching either.
	require.NotPanics(t, func() { r.reconcile(context.Background()) })
}

func TestClusterControllerBoundedChangesAndFreshness(t *testing.T) {
	node := fixtureNode()
	now := time.Now()
	for _, snapshot := range []*Snapshot{nil, {At: now.Add(-time.Minute), Active: 100000, Waiting: 100000}} {
		require.Equal(t, node.BaseWeight, DesiredWeight(node, snapshot, now), "missing telemetry falls back to configured capacity, not zero load")
	}
	fresh := &Snapshot{At: now, DBOK: true, RedisOK: true, Active: 0, AdmissionReady: true}
	require.Equal(t, node.BaseWeight, DesiredWeight(node, fresh, now))
	fresh.RedisOK = false
	require.Zero(t, DesiredWeight(node, fresh, now))
	fresh.RedisOK = true
	fresh.PendingSettlement = 128
	require.Zero(t, DesiredWeight(node, fresh, now))
	node.State = "draining"
	fresh.PendingSettlement = 0
	require.Zero(t, DesiredWeight(node, fresh, now))
}
