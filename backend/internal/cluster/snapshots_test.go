package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSnapshotsOneRoundTripPreservesUnknownAndFreshness(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runtime := &Runtime{cfg: &config.Config{Cluster: config.ClusterConfig{ID: "test"}}, redis: client}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, snapshot := range []Snapshot{{NodeID: "one", Active: 4, At: now}, {NodeID: "old", Active: 7, At: now.Add(-time.Hour)}} {
		raw, err := json.Marshal(snapshot)
		require.NoError(t, err)
		require.NoError(t, client.Set(ctx, runtime.snapshotKey(snapshot.NodeID), raw, time.Minute).Err())
	}
	before := server.CommandCount()
	got, err := runtime.Snapshots(ctx, []string{"one", "missing", "old"})
	require.NoError(t, err)
	require.Equal(t, 1, server.CommandCount()-before, "不按节点串行 GET")
	require.Equal(t, 4, got["one"].Active)
	require.Nil(t, got["missing"], "缺失不补零")
	require.False(t, got["old"].Fresh(now), "读取不刷新旧观测时间")
	require.Equal(t, "one", got["one"].NodeID)
}

func TestSnapshotReportsRealDatabasePoolWaitAndLegacyUnknown(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	held, err := db.Conn(ctx)
	require.NoError(t, err)
	defer held.Close()
	finished := make(chan error, 1)
	started := time.Now()
	go func() {
		conn, err := db.Conn(ctx)
		if err == nil {
			err = conn.Close()
		}
		finished <- err
	}()
	require.Eventually(t, func() bool {
		return db.Stats().WaitCount == 1 && time.Since(started) >= 5*time.Millisecond
	}, time.Second, time.Millisecond)
	blocked := Snapshot{BootID: "same-process"}
	blocked.setDBPoolStats(db.Stats())
	require.Equal(t, 1, blocked.DBConnections)
	require.Equal(t, 1, *blocked.DBPoolOpen)
	require.Equal(t, int64(1), *blocked.DBPoolWaitCount)
	require.NoError(t, held.Close())
	require.NoError(t, <-finished)
	ready := Snapshot{BootID: blocked.BootID}
	ready.setDBPoolStats(db.Stats())
	require.Zero(t, ready.DBConnections)
	require.Equal(t, 1, *ready.DBPoolIdle)
	require.Positive(t, *ready.DBPoolWaitMS, "completed waits must survive JSON as cumulative milliseconds")
	raw, err := json.Marshal(ready)
	require.NoError(t, err)
	var decoded Snapshot
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, ready.DBPoolWaitCount, decoded.DBPoolWaitCount)
	require.Equal(t, ready.DBPoolWaitMS, decoded.DBPoolWaitMS)
	var legacy Snapshot
	require.NoError(t, json.Unmarshal([]byte(`{"boot_id":"old-process","db_connections":0}`), &legacy))
	require.Nil(t, legacy.DBPoolWaitCount, "old nodes report unknown, not a fabricated zero")
	require.Nil(t, legacy.DBPoolWaitMS)
}

func TestSnapshotsMissingRedisEmptyAndMalformed(t *testing.T) {
	runtime := &Runtime{}
	got, err := runtime.Snapshots(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
	_, err = runtime.Snapshots(context.Background(), []string{"one"})
	require.ErrorIs(t, err, ErrNotReady)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runtime.cfg = &config.Config{Cluster: config.ClusterConfig{ID: "test"}}
	runtime.redis = client
	server.Set(runtime.snapshotKey("one"), "invalid-json")
	_, err = runtime.Snapshots(context.Background(), []string{"one"})
	require.Error(t, err)
}
