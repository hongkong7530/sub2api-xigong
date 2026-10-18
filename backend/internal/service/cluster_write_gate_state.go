package service

import "sync/atomic"

var clusterWritesAllowed atomic.Bool

func SetClusterWritesAllowed(allowed bool) {
	clusterWritesAllowed.Store(allowed)
}

func ClusterWritesAllowed() bool {
	return clusterWritesAllowed.Load()
}
