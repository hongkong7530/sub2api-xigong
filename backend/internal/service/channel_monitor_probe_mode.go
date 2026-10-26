package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

const (
	MonitorProbeModeHybrid  = "hybrid"
	MonitorProbeModePassive = "passive"
)

var ErrChannelMonitorPassive = infraerrors.BadRequest("MONITOR_PASSIVE_ONLY", "This monitor is passive-only; enable active probes before running a check")

func DefaultMonitorProbeMode(mode string) string {
	if mode == "" {
		return MonitorProbeModeHybrid
	}
	return mode
}

func validateMonitorProbeMode(mode string, groupID *int64, checkMode string) error {
	mode = DefaultMonitorProbeMode(mode)
	if mode != MonitorProbeModeHybrid && mode != MonitorProbeModePassive {
		return infraerrors.BadRequest("INVALID_MONITOR_PROBE_MODE", "probe_mode must be hybrid or passive")
	}
	if mode == MonitorProbeModePassive && (groupID == nil || *groupID <= 0 || checkMode != MonitorCheckModeProbe) {
		return infraerrors.BadRequest("PASSIVE_GROUP_REQUIRED", "Passive monitoring requires a group binding and probe check mode")
	}
	return nil
}
