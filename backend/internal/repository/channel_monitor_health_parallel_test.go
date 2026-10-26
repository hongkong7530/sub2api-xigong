package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Two independent connections must enter QueryContext before either is released.
// This verifies overlap without relying on machine-specific timing thresholds.
type monitorHealthConnector struct {
	entered chan string
	release chan struct{}
	fail    bool
}

func (d *monitorHealthConnector) Connect(context.Context) (driver.Conn, error) {
	return &monitorHealthConn{owner: d}, nil
}
func (d *monitorHealthConnector) Driver() driver.Driver { return monitorHealthDriver{} }

type monitorHealthDriver struct{}

func (monitorHealthDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type monitorHealthConn struct{ owner *monitorHealthConnector }

func (*monitorHealthConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}
func (*monitorHealthConn) Close() error              { return nil }
func (*monitorHealthConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }
func (c *monitorHealthConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	kind := "availability"
	if strings.Contains(query, "recent_facts AS") {
		kind = "recent"
	}
	c.owner.entered <- kind
	select {
	case <-c.owner.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if c.owner.fail && kind == "availability" {
		return nil, errors.New("availability failed")
	}
	if kind == "availability" {
		return &monitorHealthRows{columns: []string{"id", "model", "total", "ok", "avg"}, values: []driver.Value{int64(3), "actual", int64(2), int64(1), float64(900)}}, nil
	}
	return &monitorHealthRows{columns: []string{"id", "model", "status", "latency", "first", "rate", "at", "source"}, values: []driver.Value{int64(3), "actual", "error", int64(1200), nil, nil, time.Unix(100, 0).UTC(), "user_request"}}, nil
}

type monitorHealthRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *monitorHealthRows) Columns() []string { return r.columns }
func (*monitorHealthRows) Close() error        { return nil }
func (r *monitorHealthRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

func TestMonitorHealthCancellationStopsBothReads(t *testing.T) {
	fixture := &monitorHealthConnector{entered: make(chan string, 2), release: make(chan struct{})}
	db := sql.OpenDB(fixture)
	db.SetMaxOpenConns(2)
	defer db.Close()
	defer close(fixture.release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan monitorHealthReadResult, 1)
	go func() {
		got, err := (&channelMonitorRepository{db: db}).ReadMonitorHealth(ctx, []int64{3}, 7, 60)
		done <- monitorHealthReadResult{value: got, err: err}
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-fixture.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("health read did not start")
		}
	}
	cancel()
	select {
	case result := <-done:
		require.ErrorIs(t, result.err, context.Canceled)
		require.Nil(t, result.value)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled health reads did not return")
	}
}

func TestMonitorHealthAvailabilityOnlySkipsRecentQuery(t *testing.T) {
	fixture := &monitorHealthConnector{entered: make(chan string, 2), release: make(chan struct{})}
	close(fixture.release)
	db := sql.OpenDB(fixture)
	defer db.Close()
	got, err := (&channelMonitorRepository{db: db}).ReadMonitorHealth(context.Background(), []int64{3}, 7, 0)
	require.NoError(t, err)
	require.Equal(t, "availability", <-fixture.entered)
	require.Empty(t, fixture.entered, "no recent-facts query is needed when the limit is zero")
	require.Equal(t, 2, got[3].Availability.TotalChecks)
	require.Empty(t, got[3].Recent)
}

type monitorHealthReadResult struct {
	value map[int64]*service.ChannelMonitorHealth
	err   error
}

func TestMonitorHealthOverlapsIndependentReadsAndRejectsPartialResults(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			fixture := &monitorHealthConnector{entered: make(chan string, 2), release: make(chan struct{}), fail: fail}
			db := sql.OpenDB(fixture)
			db.SetMaxOpenConns(2)
			defer db.Close()
			defer func() {
				select {
				case <-fixture.release:
				default:
					close(fixture.release)
				}
			}()
			repo := &channelMonitorRepository{db: db}
			done := make(chan monitorHealthReadResult, 1)
			go func() {
				got, err := repo.ReadMonitorHealth(context.Background(), []int64{3}, 7, 60)
				done <- monitorHealthReadResult{value: got, err: err}
			}()
			seen := map[string]bool{}
			for i := 0; i < 2; i++ {
				select {
				case branch := <-fixture.entered:
					seen[branch] = true
				case <-time.After(3 * time.Second):
					t.Fatal("health reads were serialized")
				}
			}
			require.Len(t, seen, 2)
			close(fixture.release)
			select {
			case result := <-done:
				if fail {
					require.ErrorContains(t, result.err, "availability failed")
					require.Nil(t, result.value, "one successful source cannot publish partial health")
					return
				}
				require.NoError(t, result.err)
				require.Equal(t, 2, result.value[3].Availability.TotalChecks)
				require.Equal(t, 50.0, result.value[3].Availability.AvailabilityPct)
				require.Len(t, result.value[3].Recent, 1)
				require.Equal(t, "error", result.value[3].Recent[0].Status)
			case <-time.After(3 * time.Second):
				t.Fatal("health reads did not complete")
			}
		})
	}
}
