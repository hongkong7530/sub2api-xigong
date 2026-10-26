package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

const (
	RoleIngress     = "ingress"
	RoleWorker      = "worker"
	RoleController  = "controller"
	RoleMaintenance = "maintenance"
	RoleMigrator    = "migrator"
)

// ClusterConfig is opt-in. Disabling it preserves the existing deployment contract.
// Roles are permissions, not host-name conventions or billing run modes.
type ClusterConfig struct {
	ApplicationVersion   string   `mapstructure:"-"`
	Enabled              bool     `mapstructure:"enabled"`
	RetentionApproved    bool     `mapstructure:"retention_approved"`
	ID                   string   `mapstructure:"id"`
	NodeID               string   `mapstructure:"node_id"`
	Roles                []string `mapstructure:"roles"`
	Capabilities         []string `mapstructure:"capabilities"`
	Shadow               bool     `mapstructure:"shadow"`
	ControlURL           string   `mapstructure:"control_url"`
	ListenAddress        string   `mapstructure:"listen_address"`
	AdvertiseAddress     string   `mapstructure:"advertise_address"`
	CAFile               string   `mapstructure:"ca_file"`
	CertFile             string   `mapstructure:"cert_file"`
	KeyFile              string   `mapstructure:"key_file"`
	StateDir             string   `mapstructure:"state_dir"`
	MaxInFlight          int      `mapstructure:"max_in_flight"`
	MaxQueue             int      `mapstructure:"max_queue"`
	QueueTimeoutMS       int      `mapstructure:"queue_timeout_ms"`
	DBConnectionBudget   int      `mapstructure:"db_connection_budget"`
	HAProxyConfig        string   `mapstructure:"haproxy_config"`
	HAProxyBinary        string   `mapstructure:"haproxy_binary"`
	HAProxyService       string   `mapstructure:"haproxy_service"`
	HAProxyRuntimeSocket string   `mapstructure:"haproxy_runtime_socket"`
	ClusterBackendName   string   `mapstructure:"backend_name"`
	ShutdownSeconds      int      `mapstructure:"shutdown_seconds"`
}

var clusterIdentityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (c *Config) ClusterEnabled() bool { return c != nil && c.Cluster.Enabled }

// RunsRole is deliberately permissive only for legacy deployments.
func (c *Config) RunsRole(role string) bool {
	return !c.ClusterEnabled() || slices.Contains(c.Cluster.Roles, role)
}

func (c *Config) HasClusterCapability(capability string) bool {
	return !c.ClusterEnabled() || slices.Contains(c.Cluster.Capabilities, capability)
}

func (c *ClusterConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !clusterIdentityPattern.MatchString(c.ID) || !clusterIdentityPattern.MatchString(c.NodeID) {
		return fmt.Errorf("cluster.id and cluster.node_id must be stable lowercase identifiers")
	}
	if len(c.Roles) == 0 {
		return fmt.Errorf("cluster.roles must not be empty")
	}
	seen := map[string]bool{}
	for _, role := range c.Roles {
		if !slices.Contains([]string{RoleIngress, RoleWorker, RoleController, RoleMaintenance, RoleMigrator}, role) || seen[role] {
			return fmt.Errorf("invalid or duplicate cluster role %q", role)
		}
		seen[role] = true
	}
	if seen[RoleMigrator] && len(c.Roles) != 1 {
		return fmt.Errorf("migrator must run alone through -migrate")
	}
	if seen[RoleMigrator] {
		return nil
	}
	if !seen[RoleIngress] && !seen[RoleWorker] && !seen[RoleController] && !seen[RoleMaintenance] {
		return fmt.Errorf("no runnable cluster role")
	}
	if seen[RoleMaintenance] && !seen[RoleWorker] {
		return fmt.Errorf("maintenance requires worker for local authenticated probes")
	}
	if c.MaxInFlight < 1 || c.MaxInFlight > 100000 || c.MaxQueue < 0 || c.MaxQueue > 100000 || c.QueueTimeoutMS < 1 || c.QueueTimeoutMS > 30000 {
		return fmt.Errorf("cluster admission budget or queue timeout is invalid")
	}
	if c.DBConnectionBudget < 1 || c.ShutdownSeconds < 5 || c.ShutdownSeconds > 3600 {
		return fmt.Errorf("cluster connection budget or shutdown timeout is invalid")
	}
	if !filepath.IsAbs(c.StateDir) {
		return fmt.Errorf("cluster.state_dir must be absolute")
	}
	if c.CAFile == "" || c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("cluster mutual TLS ca_file, cert_file and key_file are required")
	}
	if _, _, err := net.SplitHostPort(c.ListenAddress); err != nil {
		return fmt.Errorf("cluster.listen_address: %w", err)
	}
	if seen[RoleWorker] {
		if _, _, err := net.SplitHostPort(c.AdvertiseAddress); err != nil {
			return fmt.Errorf("cluster.advertise_address: %w", err)
		}
	}
	if c.ControlURL != "" {
		if strings.IndexFunc(c.ControlURL, func(r rune) bool { return r <= 32 || strings.ContainsRune(";|&\"'()\\", r) || r == 36 || r == 96 }) >= 0 {
			return fmt.Errorf("cluster.control_url contains unsafe shell characters")
		}
		u, err := url.Parse(c.ControlURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("cluster.control_url must be an authenticated HTTPS origin")
		}
	}
	for _, capability := range c.Capabilities {
		if !slices.Contains([]string{"http", "websocket", "live", "plugins", "image_tasks", "batch_image"}, capability) {
			return fmt.Errorf("unknown cluster capability %q", capability)
		}
	}
	if (seen[RoleController] || seen[RoleIngress]) && !c.Shadow {
		if !filepath.IsAbs(c.HAProxyConfig) || strings.TrimSpace(c.HAProxyBinary) == "" || !clusterIdentityPattern.MatchString(c.HAProxyService) {
			return fmt.Errorf("active ingress or controller requires an absolute HAProxy config and explicit executable/service")
		}
		if !filepath.IsAbs(c.HAProxyRuntimeSocket) || !clusterIdentityPattern.MatchString(c.ClusterBackendName) {
			return fmt.Errorf("active ingress or controller requires an absolute HAProxy runtime socket and valid managed backend")
		}
	}
	return nil
}

func setClusterDefaults() {
	defaults := map[string]any{
		"retention_approved": false, "enabled": false, "id": "", "node_id": "", "roles": []string{}, "capabilities": []string{"http"}, "shadow": true,
		"control_url": "", "listen_address": "127.0.0.1:19180", "advertise_address": "", "ca_file": "", "cert_file": "", "key_file": "",
		"state_dir": "/var/lib/sub2api/cluster", "max_in_flight": 64, "max_queue": 16, "queue_timeout_ms": 1000,
		"db_connection_budget": 224, "haproxy_config": "", "haproxy_binary": "/usr/sbin/haproxy", "haproxy_service": "haproxy",
		"haproxy_runtime_socket": "/run/haproxy/admin.sock", "backend_name": "sub2api_nodes", "shutdown_seconds": 120,
	}
	for key, value := range defaults {
		viper.SetDefault("cluster."+key, value)
	}
}

// Only inspect the opt-in before legacy setup; do not bootstrap secrets or validate an uninstalled legacy instance.
func ClusterStartupEnabled() (bool, error) {
	if raw, ok := os.LookupEnv("CLUSTER_ENABLED"); ok {
		return strconv.ParseBool(raw)
	}
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	configureConfigSource(v.SetConfigFile, v.AddConfigPath)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return false, nil
		}
		return false, err
	}
	return v.GetBool("cluster.enabled"), nil
}
