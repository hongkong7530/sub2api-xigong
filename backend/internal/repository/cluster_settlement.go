package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var ErrClusterSettlementPending = errors.New("settlement is durably pending; authoritative confirmation unavailable")

type clusterSettlementJournal struct {
	reserve    *cluster.RecoveryReserve
	mu         sync.Mutex
	dir        string
	clusterID  string
	nodeID     string
	maxRecords int
}

func ProvideUsageBillingRepository(client *dbent.Client, db *sql.DB, cfg *config.Config) (service.UsageBillingRepository, error) {
	repo := &usageBillingRepository{db: db}
	if !cfg.ClusterEnabled() {
		return repo, nil
	}
	dir := filepath.Join(cfg.Cluster.StateDir, "settlements")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("settlement directory must be a real protected directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	reserve, err := cluster.NewRecoveryReserve(cfg.Cluster.StateDir, max(128, cfg.Cluster.MaxInFlight*4))
	if err != nil {
		return nil, err
	}
	repo.journal = &clusterSettlementJournal{reserve: reserve, dir: dir, clusterID: cfg.Cluster.ID, nodeID: cfg.Cluster.NodeID, maxRecords: max(128, cfg.Cluster.MaxInFlight*4)}
	return repo, nil
}

// settlementEvidence keeps billing facts and non-sensitive routing labels for usage statistics.
// Relation objects, user input, client metadata and upstream headers are excluded.
func settlementEvidence(cmd *service.UsageBillingCommand) (*service.UsageBillingCommand, error) {
	if cmd == nil || cmd.UsageLog == nil {
		return nil, fmt.Errorf("cluster settlement requires its minimal usage record")
	}
	copy := *cmd
	u := cmd.UsageLog
	copy.UsageLog = &service.UsageLog{
		ChannelID: u.ChannelID, ModelMappingChain: u.ModelMappingChain, RequestedReasoningEffort: u.RequestedReasoningEffort,
		UserID: u.UserID, APIKeyID: u.APIKeyID, AccountID: u.AccountID, RequestID: u.RequestID, Model: u.Model,
		RequestedModel: u.RequestedModel, UpstreamModel: u.UpstreamModel, GroupID: u.GroupID, SubscriptionID: u.SubscriptionID,
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CacheCreationTokens: u.CacheCreationTokens, CacheReadTokens: u.CacheReadTokens,
		CacheCreation5mTokens: u.CacheCreation5mTokens, CacheCreation1hTokens: u.CacheCreation1hTokens,
		InputCost: u.InputCost, OutputCost: u.OutputCost, CacheCreationCost: u.CacheCreationCost, CacheReadCost: u.CacheReadCost,
		ImageInputTokens: u.ImageInputTokens, ImageInputCost: u.ImageInputCost, ImageOutputTokens: u.ImageOutputTokens, ImageOutputCost: u.ImageOutputCost,
		TotalCost: u.TotalCost, ActualCost: u.ActualCost, RateMultiplier: u.RateMultiplier, AccountRateMultiplier: u.AccountRateMultiplier, AccountStatsCost: u.AccountStatsCost,
		BillingType: u.BillingType, BillingMode: u.BillingMode, BillingTier: u.BillingTier, RequestType: u.RequestType, Stream: u.Stream, OpenAIWSMode: u.OpenAIWSMode,
		ServiceTier: u.ServiceTier, ReasoningEffort: u.ReasoningEffort, DurationMs: u.DurationMs, FirstTokenMs: u.FirstTokenMs, OutputDurationMs: u.OutputDurationMs, RateObservation: u.RateObservation,
		InboundEndpoint: u.InboundEndpoint, UpstreamEndpoint: u.UpstreamEndpoint,
		ImageCount: u.ImageCount, ImageSize: u.ImageSize, ImageInputSize: u.ImageInputSize, ImageOutputSize: u.ImageOutputSize, ImageSizeSource: u.ImageSizeSource, ImageSizeBreakdown: u.ImageSizeBreakdown,
		VideoCount: u.VideoCount, VideoResolution: u.VideoResolution, VideoDurationSeconds: u.VideoDurationSeconds, MediaType: u.MediaType,
		CreatedAt: u.CreatedAt, LongContextBillingApplied: u.LongContextBillingApplied, CacheTTLOverridden: u.CacheTTLOverridden, NativeCompactionV2: u.NativeCompactionV2,
	}
	if len(copy.RequestID) > 512 || len(copy.Model) > 256 || len(u.Model) > 256 {
		return nil, fmt.Errorf("settlement identity exceeds its bound")
	}
	return &copy, nil
}

func (r *usageBillingRepository) persistClusterEvidence(ctx context.Context, cmd *service.UsageBillingCommand, data []byte) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO cluster_settlement_events(event_id,api_key_id,execution_id,cluster_id,node_id,request_fingerprint,command)
VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7::jsonb)
ON CONFLICT(event_id,api_key_id) DO UPDATE SET command=cluster_settlement_events.command
WHERE cluster_settlement_events.request_fingerprint=EXCLUDED.request_fingerprint AND cluster_settlement_events.cluster_id=EXCLUDED.cluster_id`, cmd.RequestID, cmd.APIKeyID, cmd.ExecutionID, cmd.ClusterID, cmd.NodeID, cmd.RequestFingerprint, string(data))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrUsageBillingRequestConflict
	}
	return nil
}
func (r *usageBillingRepository) lookupClusterSettlement(ctx context.Context, cmd *service.UsageBillingCommand) (bool, error) {
	var fingerprint, state string
	err := r.db.QueryRowContext(ctx, `SELECT request_fingerprint,state FROM cluster_settlement_events WHERE event_id=$1 AND api_key_id=$2 AND cluster_id=$3`, cmd.RequestID, cmd.APIKeyID, cmd.ClusterID).Scan(&fingerprint, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fingerprint != cmd.RequestFingerprint {
		return false, service.ErrUsageBillingRequestConflict
	}
	return state == "committed", nil
}
func (j *clusterSettlementJournal) file(cmd *service.UsageBillingCommand) string {
	return filepath.Join(j.dir, fmt.Sprintf("%d-%s.json", cmd.APIKeyID, service.HashUsageRequestPayload([]byte(cmd.RequestID))))
}
func (j *clusterSettlementJournal) save(cmd *service.UsageBillingCommand, data []byte) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	path := j.file(cmd)
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) != string(data) {
			return service.ErrUsageBillingRequestConflict
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return err
	}
	if len(entries) >= j.maxRecords {
		return fmt.Errorf("protected settlement journal is full")
	}
	if len(data) > 64*1024-24 {
		return fmt.Errorf("settlement evidence exceeds 64 KiB")
	}
	file, err := os.CreateTemp(j.dir, ".pending-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	if dir, err := os.Open(j.dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func (r *usageBillingRepository) applyClusterSettlement(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	return r.applyClusterSettlementWithConfirmation(ctx, context.WithoutCancel(ctx), cmd)
}

func (r *usageBillingRepository) applyClusterSettlementWithConfirmation(ctx, confirmation context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if r.journal == nil || cmd.ClusterID != r.journal.clusterID || cmd.NodeID != r.journal.nodeID {
		return nil, fmt.Errorf("cluster settlement journal not provisioned for this node")
	}
	evidence, err := settlementEvidence(cmd)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024-24 {
		return nil, fmt.Errorf("settlement evidence exceeds 64 KiB")
	}
	if err = r.persistClusterEvidence(ctx, evidence, data); err != nil {
		if errors.Is(err, service.ErrUsageBillingRequestConflict) {
			return nil, err
		}
		save := r.journal.save
		if evidence.ExecutionID != "" {
			save = func(c *service.UsageBillingCommand, b []byte) error {
				return r.journal.reserve.SaveEvent(c.ExecutionID, c.RequestID, b)
			}
		}
		if saveErr := save(evidence, data); saveErr != nil {
			return nil, fmt.Errorf("settlement evidence unavailable: %w", saveErr)
		}
		return nil, ErrClusterSettlementPending
	}
	if committed, err := r.lookupClusterSettlement(ctx, evidence); err != nil {
		return nil, err
	} else if committed {
		_ = os.Remove(r.journal.file(evidence))

		return &service.UsageBillingApplyResult{Committed: true, UsageLogPersisted: true, PlatformQuotaHandled: evidence.PlatformQuota != nil}, nil
	}
	result, applyErr := r.apply(ctx, evidence)
	if applyErr == nil && result != nil && result.Committed {
		cmd.UsageLog.ID = evidence.UsageLog.ID
		_ = os.Remove(r.journal.file(evidence))
		return result, nil
	}
	// A failed Commit may already be durable. Query the authority before deciding to retry.
	check, cancel := context.WithTimeout(confirmation, 3*time.Second)
	defer cancel()
	committed, checkErr := r.lookupClusterSettlement(check, evidence)
	if checkErr == nil && committed {
		_ = os.Remove(r.journal.file(evidence))
		if result == nil {
			result = &service.UsageBillingApplyResult{UsageLogPersisted: true, PlatformQuotaHandled: evidence.PlatformQuota != nil}
		}
		result.Committed = true
		return result, nil
	}
	if errors.Is(applyErr, service.ErrUsageBillingRequestConflict) {
		return nil, applyErr
	}
	return nil, ErrClusterSettlementPending
}

// Local evidence and the authority can describe the same monetary obligation.
// Count identities once and let an authoritative commit supersede local copies.
const clusterPendingRecoveryCountSQL = `WITH local_pending(event_id,api_key_id) AS (
 SELECT * FROM unnest($3::text[],$4::bigint[])
), outstanding AS (
 SELECT event_id,api_key_id FROM cluster_settlement_events
 WHERE cluster_id=$1 AND node_id=$2 AND state<>'committed'
 UNION
 SELECT l.event_id,l.api_key_id FROM local_pending l
 WHERE NOT EXISTS(SELECT 1 FROM cluster_settlement_events e
  WHERE e.cluster_id=$1 AND e.event_id=l.event_id AND e.api_key_id=l.api_key_id AND e.state='committed')
)
SELECT count(*) FROM outstanding`

// RecoverClusterSettlements replays only immutable money commands; it never performs model calls.
func (r *usageBillingRepository) RecoverClusterSettlements(ctx context.Context) (int, error) {
	if r.journal == nil {
		return 0, nil
	}
	if err := r.journal.checkWritable(); err != nil {
		return 1, err
	}
	reserved, err := r.journal.reserve.ReadAll()
	if err != nil {
		return 1, err
	}
	completions := make(map[string]string)
	reservedIDs := make([]string, 0, len(reserved))
	pending := 0
	type eventIdentity struct {
		eventID  string
		apiKeyID int64
	}
	localPending := make(map[eventIdentity]struct{})
	rememberPending := func(cmd *service.UsageBillingCommand) {
		localPending[eventIdentity{cmd.RequestID, cmd.APIKeyID}] = struct{}{}
	}
	// Monetary replay may wait on a remote hot row. Keep half of this cycle for
	// independent authority reads; deferring durable evidence is not a commit.
	budget := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, max(time.Duration(0), time.Until(deadline)/2))
	}
	replayCtx, cancelReplay := context.WithTimeout(ctx, budget)
	defer cancelReplay()
	deferred := make(map[eventIdentity]string)
	replay := func(cmd *service.UsageBillingCommand) error {
		if cmd.ClusterID != r.journal.clusterID || cmd.NodeID != r.journal.nodeID {
			return fmt.Errorf("cluster settlement journal not provisioned for this node")
		}
		evidence, err := settlementEvidence(cmd)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(evidence)
		if err != nil || len(encoded) > 64*1024-24 {
			return fmt.Errorf("invalid bounded settlement evidence")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		identity := eventIdentity{cmd.RequestID, cmd.APIKeyID}
		if prior, ok := deferred[identity]; ok && prior != cmd.RequestFingerprint {
			return service.ErrUsageBillingRequestConflict
		}
		if replayCtx.Err() == nil {
			// Recovery confirmation shares the replay deadline; ordinary requests
			// still retain their detached post-commit authority check.
			_, err = r.applyClusterSettlementWithConfirmation(replayCtx, replayCtx, cmd)
			if err == nil || errors.Is(err, service.ErrUsageBillingRequestConflict) {
				return err
			}
			if replayCtx.Err() == nil || (!errors.Is(err, ErrClusterSettlementPending) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
				return err
			}
		}
		deferred[identity] = cmd.RequestFingerprint
		return ErrClusterSettlementPending
	}
	for id, data := range reserved {
		replayPending := false
		var envelope cluster.RecoveryEvents
		if len(data) > 0 {
			if json.Unmarshal(data, &envelope) != nil || envelope.Version != 1 {
				return 1, fmt.Errorf("invalid reserved settlement envelope")
			}
			if envelope.FinishState != "" && !validRecoveryCompletionState(envelope.FinishState) {
				return pending + 1, fmt.Errorf("invalid execution completion state")
			}
			for event, record := range envelope.Events {
				var command service.UsageBillingCommand
				if json.Unmarshal(record, &command) != nil || command.ExecutionID != id || command.RequestID != event {
					return 1, fmt.Errorf("recovery slot identity mismatch")
				}
				if err := replay(&command); err != nil {
					if !errors.Is(err, ErrClusterSettlementPending) {
						return pending + 1, err
					}
					// Durable evidence remains replayable; count the obligation without
					// turning an unconfirmed commit into a whole-node health failure.
					pending++
					rememberPending(&command)
					replayPending = true
				}
			}
		}
		// A completion marker cannot release a slot while any of its money
		// commands still lacks authoritative confirmation.
		if replayPending {
			continue
		}
		reservedIDs = append(reservedIDs, id)
		if envelope.FinishState != "" {
			completions[id] = envelope.FinishState
		}
	}
	if err := r.finishRecoveredExecutions(ctx, completions, reservedIDs); err != nil {
		return 1, err
	}
	entries, err := os.ReadDir(r.journal.dir)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		pending++
		path := filepath.Join(r.journal.dir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return pending, fmt.Errorf("symlink recovery record rejected")
		}
		info, err := entry.Info()
		if err != nil {
			return pending, err
		}
		if info.Size() > 64*1024 {
			return pending, fmt.Errorf("oversized recovery record")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return pending, err
		}
		var cmd service.UsageBillingCommand
		if err = json.Unmarshal(data, &cmd); err != nil {
			return pending, fmt.Errorf("invalid settlement record; preserved for review")
		}
		if filepath.Base(r.journal.file(&cmd)) != entry.Name() {
			return pending, fmt.Errorf("settlement record identity mismatch")
		}
		if err = replay(&cmd); err != nil {
			if errors.Is(err, ErrClusterSettlementPending) {
				rememberPending(&cmd)
				continue
			}
			return pending, err
		}
		pending--
	}
	rows, err := r.db.QueryContext(ctx, `SELECT command FROM cluster_settlement_events WHERE cluster_id=$1 AND node_id=$2 AND state='pending' ORDER BY created_at LIMIT 32`, r.journal.clusterID, r.journal.nodeID)
	if err != nil {
		return pending, err
	}
	var batch [][]byte
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			_ = rows.Close()
			return pending, err
		}
		batch = append(batch, data)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return pending, err
	}
	for _, data := range batch {
		var cmd service.UsageBillingCommand
		if json.Unmarshal(data, &cmd) != nil {
			return pending + 1, fmt.Errorf("invalid persisted settlement")
		}
		if err = replay(&cmd); err != nil {
			if !errors.Is(err, ErrClusterSettlementPending) {
				return pending + 1, err
			}
			rememberPending(&cmd)
		}
	}
	// Reconcile before counting: admission is closed by the unsettled count, so stale
	// rows must be resolved rather than merely reported.
	reconciled, err := r.reconcileStaleExecutions(ctx, reserved, clusterStaleExecutionGrace, clusterStaleExecutionBatch)
	if err != nil {
		return pending + 1, err
	}
	_ = reconciled
	if len(deferred) > 0 {
		ids := make([]string, 0, len(deferred))
		keys := make([]int64, 0, len(deferred))
		fingerprints := make([]string, 0, len(deferred))
		for identity, fingerprint := range deferred {
			ids = append(ids, identity.eventID)
			keys = append(keys, identity.apiKeyID)
			fingerprints = append(fingerprints, fingerprint)
		}
		// A budget deferral must not hide a conflicting authority identity.
		var conflict bool
		err := r.db.QueryRowContext(ctx, `WITH deferred(event_id,api_key_id,fingerprint) AS (
			SELECT * FROM unnest($2::text[],$3::bigint[],$4::text[])
		) SELECT EXISTS(SELECT 1 FROM deferred d JOIN cluster_settlement_events e
			ON e.event_id=d.event_id AND e.api_key_id=d.api_key_id
			WHERE e.cluster_id<>$1 OR e.request_fingerprint<>d.fingerprint)`,
			r.journal.clusterID, pq.Array(ids), pq.Array(keys), pq.Array(fingerprints)).Scan(&conflict)
		if err != nil {
			return pending + 1, err
		}
		if conflict {
			return pending + 1, service.ErrUsageBillingRequestConflict
		}
	}
	eventIDs := make([]string, 0, len(localPending))
	apiKeyIDs := make([]int64, 0, len(localPending))
	for identity := range localPending {
		eventIDs = append(eventIDs, identity.eventID)
		apiKeyIDs = append(apiKeyIDs, identity.apiKeyID)
	}
	var count int
	err = r.db.QueryRowContext(ctx, clusterPendingRecoveryCountSQL, r.journal.clusterID, r.journal.nodeID, pq.Array(eventIDs), pq.Array(apiKeyIDs)).Scan(&count)
	return count, err
}

func validRecoveryCompletionState(state string) bool {
	return state == "rejected" || state == "needs_review" || state == "awaiting_settlement"
}

// finishRecoveredExecutions replays only known completion markers, then releases
// slots already terminal in the authority. Its query count is independent of slot count.
func (r *usageBillingRepository) finishRecoveredExecutions(ctx context.Context, completions map[string]string, reservedIDs []string) error {
	if len(completions) > 0 {
		ids := make([]string, 0, len(completions))
		states := make([]string, 0, len(completions))
		for id, state := range completions {
			if !validRecoveryCompletionState(state) {
				return fmt.Errorf("invalid execution completion state")
			}
			ids = append(ids, id)
			states = append(states, state)
		}
		result, err := r.db.ExecContext(ctx, `
			UPDATE cluster_executions x SET work_finished=TRUE,
			state=CASE WHEN x.state IN ('settled','rejected') THEN x.state
				WHEN c.finish_state='rejected' THEN 'rejected'
				WHEN EXISTS(SELECT 1 FROM cluster_settlement_events e WHERE e.execution_id=x.execution_id)
					AND NOT EXISTS(SELECT 1 FROM cluster_settlement_events e WHERE e.execution_id=x.execution_id AND e.state<>'committed') THEN 'settled'
				ELSE c.finish_state END, updated_at=NOW()
			FROM unnest($1::uuid[],$2::text[]) AS c(execution_id,finish_state)
			WHERE x.execution_id=c.execution_id AND x.cluster_id=$3 AND x.node_id=$4`, pq.Array(ids), pq.Array(states), r.journal.clusterID, r.journal.nodeID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != int64(len(completions)) {
			return fmt.Errorf("execution completion identity not found")
		}
	}
	if len(reservedIDs) == 0 {
		return nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT execution_id::text FROM cluster_executions
		WHERE cluster_id=$1 AND node_id=$2 AND execution_id=ANY($3::uuid[])
			AND state IN ('settled','rejected')`, r.journal.clusterID, r.journal.nodeID, pq.Array(reservedIDs))
	if err != nil {
		return err
	}
	var terminal []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		terminal = append(terminal, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range terminal {
		if err := r.journal.reserve.ReleaseAfterReplay(id); err != nil {
			return err
		}
	}
	return nil
}

const (
	// A dispatched request that ended with an error keeps state needs_review on purpose:
	// its monetary result is unconfirmed. Those rows must still be resolved eventually,
	// because the admission gate counts unsettled executions and would otherwise refuse
	// every new request once the backlog reaches the ceiling.
	clusterStaleExecutionGrace = 10 * time.Minute
	clusterStaleExecutionBatch = 64
)

// reconcileStaleExecutions decides finished executions whose money has already been
// settled by whichever path actually ran, and which therefore must stop blocking
// admission. The verdict comes from the ledger, never from a guess: deduction and its
// ledger row commit in one transaction, so a missing row proves no money was taken.
// Executions still carrying replayable evidence are skipped and left to the replay
// loop above, and only executions older than the grace window are considered so live
// requests are never raced.
func (r *usageBillingRepository) reconcileStaleExecutions(ctx context.Context, pendingEvidence map[string][]byte, grace time.Duration, limit int) (int, error) {
	if r.journal == nil || limit <= 0 {
		return 0, nil
	}
	// A completion-only envelope proves that work ended; it is not an unreplayed
	// money command. Monetary evidence still excludes an execution from this pass.
	monetaryIDs := make([]string, 0, len(pendingEvidence))
	for id, data := range pendingEvidence {
		if len(data) == 0 {
			continue
		}
		var envelope cluster.RecoveryEvents
		if json.Unmarshal(data, &envelope) != nil || envelope.Version != 1 {
			return 0, fmt.Errorf("invalid reserved settlement envelope")
		}
		if len(envelope.Events) > 0 {
			monetaryIDs = append(monetaryIDs, id)
		}
	}
	// Decide the bounded batch in one database round trip. Per-execution queries
	// exhaust the recovery deadline over a cross-region database connection.
	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT execution_id
			FROM cluster_executions
			WHERE cluster_id=$1 AND node_id=$2
				AND state IN ('needs_review','awaiting_settlement')
				AND work_finished
				AND created_at < NOW() - make_interval(secs => $3)
				AND NOT (execution_id = ANY($5::uuid[]))
				AND NOT EXISTS (
					SELECT 1 FROM cluster_settlement_events e
					WHERE e.execution_id=cluster_executions.execution_id AND e.state<>'committed')
			ORDER BY created_at
			LIMIT $4
			FOR UPDATE
		)
		UPDATE cluster_executions x
		SET state=CASE WHEN EXISTS (
			SELECT 1 FROM cluster_settlement_events e
			JOIN billing_usage_entries b
				ON b.settlement_event_id=e.event_id AND b.api_key_id=e.api_key_id
			WHERE e.execution_id=x.execution_id AND e.state='committed'
				AND (b.applied OR b.delta_usd<>0)
			UNION ALL
			SELECT 1 FROM usage_logs l
			JOIN billing_usage_entries b ON b.usage_log_id=l.id
			WHERE l.request_id='execution:'||x.execution_id::text AND (b.applied OR b.delta_usd<>0)
		) THEN 'settled' ELSE 'rejected' END,
		work_finished=TRUE, updated_at=NOW()
		FROM candidates c
		WHERE x.execution_id=c.execution_id
			AND x.cluster_id=$1 AND x.node_id=$2
			AND x.state IN ('needs_review','awaiting_settlement')
		RETURNING x.execution_id::text`, r.journal.clusterID, r.journal.nodeID, int(grace.Seconds()), limit, pq.Array(monetaryIDs))
	if err != nil {
		return 0, err
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		candidates = append(candidates, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, id := range candidates {
		if err := r.journal.reserve.ReleaseAfterReplay(id); err != nil {
			return resolved, err
		}
		resolved++
	}
	return resolved, nil
}

// Failure blocks admission before a known result needs its fallback journal.
func (j *clusterSettlementJournal) checkWritable() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return err
	}
	if len(entries) >= j.maxRecords {
		return fmt.Errorf("settlement journal capacity exhausted")
	}
	probe, err := os.CreateTemp(j.dir, ".write-check-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	defer os.Remove(name)
	if err = probe.Chmod(0600); err == nil {
		_, err = probe.Write([]byte("ready"))
	}
	if err == nil {
		err = probe.Sync()
	}
	closeErr := probe.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// The runtime and settlement owner share one capacity lock for this process.
func (r *usageBillingRepository) ClusterRecoveryReserve() *cluster.RecoveryReserve {
	if r.journal == nil {
		return nil
	}
	return r.journal.reserve
}
