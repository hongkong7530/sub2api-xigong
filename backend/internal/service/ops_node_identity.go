package service

import (
	"os"
	"regexp"
	"strings"
)

var opsNodeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type OpsNodeIdentity struct {
	ID   string
	Name string
}

func LoadOpsNodeIdentity() OpsNodeIdentity {
	nodeID := strings.ToLower(strings.TrimSpace(os.Getenv("OPS_NODE_ID")))
	if !opsNodeIDPattern.MatchString(nodeID) {
		if hostname, err := os.Hostname(); err == nil {
			nodeID = strings.ToLower(strings.TrimSpace(hostname))
		}
	}
	if !opsNodeIDPattern.MatchString(nodeID) {
		nodeID = "unknown"
	}
	nodeName := strings.TrimSpace(os.Getenv("OPS_NODE_NAME"))
	if nodeName == "" {
		nodeName = nodeID
	}
	return OpsNodeIdentity{ID: nodeID, Name: truncateString(nodeName, 128)}
}
