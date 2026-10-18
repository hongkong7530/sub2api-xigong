package service

import "sync/atomic"

var (
	clusterActiveRequests  atomic.Int64
	clusterTotalRequests   atomic.Uint64
	clusterWaitingRequests atomic.Int64
)

func IncrementClusterActiveRequests() {
	clusterActiveRequests.Add(1)
	clusterTotalRequests.Add(1)
}

func DecrementClusterActiveRequests() {
	clusterActiveRequests.Add(-1)
}

func ClusterActiveRequests() int {
	value := clusterActiveRequests.Load()
	if value < 0 {
		return 0
	}
	return int(value)
}

func ClusterTotalRequests() uint64 {
	return clusterTotalRequests.Load()
}
