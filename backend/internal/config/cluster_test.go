package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func validClusterConfig() ClusterConfig {
	return ClusterConfig{Enabled: true, ID: "test", NodeID: "worker-1", Roles: []string{RoleWorker}, Capabilities: []string{"http"}, Shadow: true, ListenAddress: "127.0.0.1:19180", AdvertiseAddress: "127.0.0.1:19180", CAFile: "ca.pem", CertFile: "node.pem", KeyFile: "node.key", StateDir: "C:/cluster-state", MaxInFlight: 8, MaxQueue: 2, QueueTimeoutMS: 1000, DBConnectionBudget: 32, ShutdownSeconds: 60}
}
func TestClusterRolesDefaultAndStrict(t *testing.T) {
	var cfg *Config
	require.True(t, cfg.RunsRole(RoleMaintenance))
	c := validClusterConfig()
	// Paths are tested on the host platform instead of assuming Unix during Windows builds.
	c.StateDir = t.TempDir()
	require.NoError(t, c.Validate())
	controller := c
	controller.Roles = []string{RoleController}
	controller.Shadow = false
	controller.HAProxyConfig = filepath.Join(t.TempDir(), "haproxy.cfg")
	controller.HAProxyBinary = "/usr/sbin/haproxy"
	controller.HAProxyService = "haproxy"
	controller.HAProxyRuntimeSocket = filepath.Join(t.TempDir(), "admin.sock")
	controller.ClusterBackendName = "sub2api_nodes"
	require.NoError(t, controller.Validate())
	cfg = &Config{Cluster: c}
	require.True(t, cfg.RunsRole(RoleWorker))
	require.False(t, cfg.RunsRole(RoleController))
	c.Roles = nil
	require.Error(t, c.Validate())
	c.Roles = []string{RoleWorker, RoleWorker}
	require.Error(t, c.Validate())
	c.Roles = []string{RoleMigrator, RoleWorker}
	require.Error(t, c.Validate())
	c.Roles = []string{RoleMigrator}
	require.NoError(t, c.Validate())
}
func TestClusterConfigurationRejectsUnsafeBoundaries(t *testing.T) {
	for _, mutate := range []func(*ClusterConfig){
		func(c *ClusterConfig) { c.NodeID = "unknown/other" },
		func(c *ClusterConfig) { c.CAFile = "" },
		func(c *ClusterConfig) { c.MaxInFlight = 0 },
		func(c *ClusterConfig) { c.ControlURL = "http://controller" },
		func(c *ClusterConfig) {
			c.Roles = []string{RoleController}
			c.Shadow = false
			c.HAProxyRuntimeSocket = ""
		},
		func(c *ClusterConfig) {
			c.Roles = []string{RoleController}
			c.Shadow = false
			c.ClusterBackendName = "invalid/backend"
		},
		func(c *ClusterConfig) { c.Roles = []string{RoleMaintenance} },
		func(c *ClusterConfig) { c.Capabilities = []string{"unverified"} },
	} {
		c := validClusterConfig()
		c.StateDir = t.TempDir()
		mutate(&c)
		require.Error(t, c.Validate())
	}
}
