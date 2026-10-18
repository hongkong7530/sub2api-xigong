package cluster

import (
	"testing"
	"time"

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
