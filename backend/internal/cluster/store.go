package cluster

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	DB               *sql.DB
	ClusterID        string
	ConnectionBudget int
}

const nodeColumns = `cluster_id,node_id,name,roles,capabilities,address,certificate_sha256,state,revision,base_weight,max_in_flight,max_queue,db_connections,monthly_limit_bytes,traffic_reserve_bytes,traffic_cycle_day,traffic_timezone,traffic_source,version,applied_revision,COALESCE(apply_error,'')`

type rowScanner interface{ Scan(...any) error }

func scanNode(row rowScanner) (*Node, error) {
	var n Node
	var roles, capabilities []byte
	err := row.Scan(&n.ClusterID, &n.ID, &n.Name, &roles, &capabilities, &n.Address, &n.CertificateSHA256, &n.State, &n.Revision, &n.BaseWeight, &n.MaxInFlight, &n.MaxQueue, &n.DBConnections, &n.MonthlyLimitBytes, &n.TrafficReserveBytes, &n.TrafficCycleDay, &n.TrafficTimezone, &n.TrafficSource, &n.Version, &n.AppliedRevision, &n.ApplyError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(roles, &n.Roles); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(capabilities, &n.Capabilities); err != nil {
		return nil, err
	}
	return &n, nil
}
func (s *Store) Node(ctx context.Context, id string) (*Node, error) {
	return scanNode(s.DB.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM cluster_nodes WHERE cluster_id=$1 AND node_id=$2`, s.ClusterID, id))
}
func (s *Store) Nodes(ctx context.Context) ([]Node, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+nodeColumns+` FROM cluster_nodes WHERE cluster_id=$1 ORDER BY node_id`, s.ClusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// Register reserves connection capacity and certificate identity before issuing a one-use invitation.
// No database or Redis credentials are embedded in the invitation or returned by this API.
func (s *Store) Register(ctx context.Context, n Node, actor int64) (string, error) {
	n.ClusterID = s.ClusterID
	n.State = "pending"
	n.Revision = 1
	if err := n.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if actor <= 0 {
		return "", ErrForbidden
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO cluster_control(cluster_id) VALUES($1) ON CONFLICT DO NOTHING`, s.ClusterID); err != nil {
		return "", err
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM cluster_control WHERE cluster_id=$1 FOR UPDATE`, s.ClusterID).Scan(&revision); err != nil {
		return "", err
	}
	var allocated int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(db_connections),0) FROM cluster_nodes WHERE cluster_id=$1 AND state<>'retired'`, s.ClusterID).Scan(&allocated); err != nil {
		return "", err
	}
	if allocated+n.DBConnections > s.ConnectionBudget {
		return "", fmt.Errorf("%w: cluster database connection budget exceeded: other nodes use %d, requested %d, total budget %d; database pool connections are not request concurrency", ErrInvalid, allocated, n.DBConnections, s.ConnectionBudget)
	}
	var priorState string
	priorErr := tx.QueryRowContext(ctx, `SELECT state FROM cluster_nodes WHERE cluster_id=$1 AND node_id=$2 FOR UPDATE`, s.ClusterID, n.ID).Scan(&priorState)
	if priorErr == nil {
		if priorState != "retired" {
			return "", ErrConflict
		}
		var liveCredentials bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cluster_node_credentials WHERE cluster_id=$1 AND node_id=$2 AND status<>'revoked')`, s.ClusterID, n.ID).Scan(&liveCredentials); err != nil {
			return "", err
		}
		if liveCredentials {
			return "", ErrUnsafeExit
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM cluster_join_tokens WHERE cluster_id=$1 AND node_id=$2`, s.ClusterID, n.ID); err != nil {
			return "", err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM cluster_node_credentials WHERE cluster_id=$1 AND node_id=$2 AND status='revoked'`, s.ClusterID, n.ID); err != nil {
			return "", err
		}
	} else if !errors.Is(priorErr, sql.ErrNoRows) {
		return "", priorErr
	}
	roles, _ := json.Marshal(n.Roles)
	caps, _ := json.Marshal(n.Capabilities)
	_, err = tx.ExecContext(ctx, `INSERT INTO cluster_nodes (cluster_id,node_id,name,roles,capabilities,address,certificate_sha256,state,revision,base_weight,max_in_flight,max_queue,db_connections,monthly_limit_bytes,traffic_reserve_bytes,traffic_cycle_day,traffic_timezone,traffic_source,version) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',1,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(cluster_id,node_id) DO UPDATE SET name=EXCLUDED.name,roles=EXCLUDED.roles,capabilities=EXCLUDED.capabilities,address=EXCLUDED.address,certificate_sha256=EXCLUDED.certificate_sha256,state='pending',revision=cluster_nodes.revision+1,applied_revision=0,apply_error=NULL,base_weight=EXCLUDED.base_weight,max_in_flight=EXCLUDED.max_in_flight,max_queue=EXCLUDED.max_queue,db_connections=EXCLUDED.db_connections,monthly_limit_bytes=EXCLUDED.monthly_limit_bytes,traffic_reserve_bytes=EXCLUDED.traffic_reserve_bytes,traffic_cycle_day=EXCLUDED.traffic_cycle_day,traffic_timezone=EXCLUDED.traffic_timezone,traffic_source=EXCLUDED.traffic_source,version=EXCLUDED.version,updated_at=NOW()`, s.ClusterID, n.ID, n.Name, roles, caps, n.Address, n.CertificateSHA256, n.BaseWeight, n.MaxInFlight, n.MaxQueue, n.DBConnections, n.MonthlyLimitBytes, n.TrafficReserveBytes, n.TrafficCycleDay, n.TrafficTimezone, n.TrafficSource, n.Version)
	if err != nil {
		return "", err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	_, err = tx.ExecContext(ctx, `INSERT INTO cluster_join_tokens(token_sha256,cluster_id,node_id,expires_at,created_by) VALUES($1,$2,$3,NOW()+INTERVAL '15 minutes',$4)`, fingerprint(token), s.ClusterID, n.ID, actor)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cluster_control SET revision=revision+1,updated_at=NOW() WHERE cluster_id=$1`, s.ClusterID); err != nil {
		return "", err
	}
	var targetRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM cluster_nodes WHERE cluster_id=$1 AND node_id=$2`, s.ClusterID, n.ID).Scan(&targetRevision); err != nil {
		return "", err
	}
	operationID := uuid.NewString()
	if _, err = tx.ExecContext(ctx, `INSERT INTO cluster_operations(id,cluster_id,node_id,request_key,request_fingerprint,action,expected_revision,target_revision,status,actor_user_id) VALUES($1,$2,$3,$1,$4,'register',$5,$6,'registered',$7)`, operationID, s.ClusterID, n.ID, fingerprint(n), targetRevision-1, targetRevision, actor); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Enroll(ctx context.Context, id, certificate, token string) (*Node, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	n, err := scanNode(tx.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM cluster_nodes WHERE cluster_id=$1 AND node_id=$2 FOR UPDATE`, s.ClusterID, id))
	if err != nil {
		return nil, err
	}
	if n.State != "pending" || n.CertificateSHA256 != certificate {
		return nil, ErrForbidden
	}
	result, err := tx.ExecContext(ctx, `UPDATE cluster_join_tokens SET consumed_at=NOW() WHERE token_sha256=$1 AND cluster_id=$2 AND node_id=$3 AND consumed_at IS NULL AND expires_at>NOW()`, fingerprint(token), s.ClusterID, id)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrForbidden
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Store) Authenticate(ctx context.Context, id, certificate string) (*Node, error) {
	n, err := s.Node(ctx, id)
	if err != nil {
		return nil, err
	}
	if n.State == "retired" || n.CertificateSHA256 != certificate {
		return nil, ErrForbidden
	}
	var enrolled bool
	err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cluster_join_tokens WHERE cluster_id=$1 AND node_id=$2 AND consumed_at IS NOT NULL)`, s.ClusterID, id).Scan(&enrolled)
	if err != nil {
		return nil, err
	}
	if !enrolled {
		return nil, ErrForbidden
	}
	return n, nil
}

func scanOperation(row rowScanner) (*Operation, error) {
	var op Operation
	err := row.Scan(&op.ID, &op.NodeID, &op.RequestKey, &op.Action, &op.TargetRevision, &op.Status, &op.ErrorCode, &op.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &op, err
}

const operationColumns = `id::text,node_id,request_key::text,action,target_revision,status,COALESCE(error_code,''),created_at`

func (s *Store) Operations(ctx context.Context) ([]Operation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+operationColumns+` FROM cluster_operations WHERE cluster_id=$1 ORDER BY created_at DESC LIMIT 100`, s.ClusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		v, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (s *Store) Change(ctx context.Context, id string, c Change, actor int64, live *Snapshot) (*Operation, error) {
	if _, err := uuid.Parse(c.RequestKey); err != nil {
		return nil, fmt.Errorf("%w: request_key must be a UUID", ErrInvalid)
	}
	if actor <= 0 {
		return nil, ErrForbidden
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM cluster_control WHERE cluster_id=$1 FOR UPDATE`, s.ClusterID).Scan(&revision); err != nil {
		return nil, err
	}
	fp := fingerprint(struct {
		ID     string
		Change Change
		Actor  int64
	}{id, c, actor})
	var storedFP string
	err = tx.QueryRowContext(ctx, `SELECT request_fingerprint FROM cluster_operations WHERE cluster_id=$1 AND request_key=$2`, s.ClusterID, c.RequestKey).Scan(&storedFP)
	if err == nil {
		if storedFP != fp {
			return nil, ErrConflict
		}
		return scanOperation(tx.QueryRowContext(ctx, `SELECT `+operationColumns+` FROM cluster_operations WHERE cluster_id=$1 AND request_key=$2`, s.ClusterID, c.RequestKey))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	n, err := scanNode(tx.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM cluster_nodes WHERE cluster_id=$1 AND node_id=$2 FOR UPDATE`, s.ClusterID, id))
	if err != nil {
		return nil, err
	}
	if c.Action == "configure" && c.Node != nil && c.Node.Version != n.Version {
		return nil, fmt.Errorf("%w: version changes require reenrollment", ErrInvalid)
	}
	if n.Revision != c.ExpectedRevision || n.State == "retired" {
		return nil, ErrConflict
	}
	switch c.Action {
	case "activate", "resume":
		if live == nil || !live.Fresh(time.Now()) || !live.DBOK || !live.RedisOK || live.PendingSettlement != 0 {
			return nil, ErrNotReady
		}
		var incompatible int
		capabilities, _ := json.Marshal(n.Capabilities)
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cluster_nodes WHERE cluster_id=$1 AND node_id<>$2 AND state='ready' AND roles @> '["worker"]'::jsonb AND NOT (capabilities @> $3::jsonb AND capabilities <@ $3::jsonb)`, s.ClusterID, id, capabilities).Scan(&incompatible); err != nil {
			return nil, err
		}
		if incompatible > 0 {
			return nil, fmt.Errorf("%w: shared worker pool requires the same verified capability set", ErrInvalid)
		}
		n.State = "ready"
	case "reapply":
		// A new revision permits retry after a previously rejected proxy configuration.
	case "pause":
		n.State = "paused"
	case "drain":
		n.State = "draining"
	case "retire":
		var unfinished int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cluster_executions WHERE cluster_id=$1 AND node_id=$2 AND state NOT IN ('settled','rejected')`, s.ClusterID, id).Scan(&unfinished); err != nil {
			return nil, err
		}
		if unfinished > 0 {
			return nil, ErrUnsafeExit
		}
		if n.AppliedRevision != n.Revision {
			return nil, ErrUnsafeExit
		}
		if n.State != "draining" || live == nil || !live.Fresh(time.Now()) || live.Active != 0 || live.Waiting != 0 || live.PendingSettlement != 0 || live.ObservedRevision < n.Revision {
			return nil, ErrUnsafeExit
		}
		n.State = "retired"
	case "force_retire":
		if !c.ConfirmForce {
			return nil, ErrUnsafeExit
		}
		n.State = "retired"
		if _, err = tx.ExecContext(ctx, `UPDATE cluster_executions SET state='needs_review',updated_at=NOW() WHERE cluster_id=$1 AND node_id=$2 AND state IN ('accepted','executing','awaiting_settlement')`, s.ClusterID, id); err != nil {
			return nil, err
		}
	case "configure":
		if c.Node == nil {
			return nil, fmt.Errorf("%w: node configuration required", ErrInvalid)
		}
		updated := *c.Node
		updated.ClusterID = s.ClusterID
		updated.ID = id
		updated.State = n.State
		updated.Revision = n.Revision
		updated.CertificateSHA256 = n.CertificateSHA256
		if err := updated.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		// Changing endpoint, identity or execution roles requires a new enrollment, not an unverified edit.
		if updated.Address != n.Address || fingerprint(updated.Roles) != fingerprint(n.Roles) || fingerprint(updated.Capabilities) != fingerprint(n.Capabilities) {
			return nil, fmt.Errorf("%w: endpoint, role and capability changes require reenrollment", ErrInvalid)
		}
		var allocated int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(db_connections),0) FROM cluster_nodes WHERE cluster_id=$1 AND node_id<>$2 AND state<>'retired'`, s.ClusterID, id).Scan(&allocated); err != nil {
			return nil, err
		}
		if allocated+updated.DBConnections > s.ConnectionBudget {
			return nil, fmt.Errorf("%w: cluster database connection budget exceeded: other nodes use %d, requested %d, total budget %d; database pool connections are not request concurrency", ErrInvalid, allocated, updated.DBConnections, s.ConnectionBudget)
		}
		n = &updated
	default:
		return nil, fmt.Errorf("%w: unsupported node operation", ErrInvalid)
	}
	_, err = tx.ExecContext(ctx, `UPDATE cluster_nodes SET name=$3,state=$4,revision=revision+1,base_weight=$5,max_in_flight=$6,max_queue=$7,db_connections=$8,monthly_limit_bytes=$9,traffic_reserve_bytes=$10,traffic_cycle_day=$11,traffic_timezone=$12,traffic_source=$13,updated_at=NOW() WHERE cluster_id=$1 AND node_id=$2`, s.ClusterID, id, n.Name, n.State, n.BaseWeight, n.MaxInFlight, n.MaxQueue, n.DBConnections, n.MonthlyLimitBytes, n.TrafficReserveBytes, n.TrafficCycleDay, n.TrafficTimezone, n.TrafficSource)
	if err != nil {
		return nil, err
	}
	op := Operation{ID: uuid.NewString(), NodeID: id, RequestKey: c.RequestKey, Action: c.Action, TargetRevision: n.Revision + 1, Status: "pending", CreatedAt: time.Now().UTC()}
	_, err = tx.ExecContext(ctx, `INSERT INTO cluster_operations(id,cluster_id,node_id,request_key,request_fingerprint,action,expected_revision,target_revision,actor_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, op.ID, s.ClusterID, id, c.RequestKey, fp, c.Action, c.ExpectedRevision, op.TargetRevision, actor)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cluster_control SET revision=revision+1,updated_at=NOW() WHERE cluster_id=$1`, s.ClusterID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &op, nil
}

// OperationByRequestKey resolves an uncertain submission without repeating its side effects.
func (s *Store) OperationByRequestKey(ctx context.Context, key string) (*Operation, error) {
	if _, err := uuid.Parse(key); err != nil {
		return nil, fmt.Errorf("%w: request_key must be a UUID", ErrInvalid)
	}
	return scanOperation(s.DB.QueryRowContext(ctx, `SELECT `+operationColumns+` FROM cluster_operations WHERE cluster_id=$1 AND request_key=$2`, s.ClusterID, key))
}
