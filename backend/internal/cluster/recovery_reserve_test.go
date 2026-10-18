package cluster

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClusterRecoveryReserveCapacityAndConfirmation(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 1)
	require.NoError(t, err)
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.Error(t, r.Reserve(uuid.NewString()))
	entries, err := r.ReadAll()
	require.NoError(t, err)
	require.Contains(t, entries, id)
	require.Empty(t, entries[id])
	require.NoError(t, r.Save(id, []byte(`{"event":"first"}`)))
	require.NoError(t, r.Save(id, []byte(`{"event":"second"}`)))
	entries, err = r.ReadAll()
	require.NoError(t, err)
	require.JSONEq(t, `{"event":"second"}`, string(entries[id]))
	// Evidence still present: removal must be refused, never performed, because a caller's
	// earlier "empty slot" reading is a snapshot and cannot authorise a delete.
	require.Error(t, r.Confirm(id))
	entries, err = r.ReadAll()
	require.NoError(t, err)
	require.JSONEq(t, `{"event":"second"}`, string(entries[id]))
	// Once the evidence has been replayed away the slot carries no valid frame again, so
	// removal succeeds. Save rejects an empty payload, so the cleared state is written
	// directly at the frame level, which is exactly what the writer produces for a
	// freshly reserved slot.
	require.NoError(t, r.Save(id, []byte(`{"event":"replayed"}`)))
	clearSlotFrame(t, filepath.Join(r.dir, id+".slot"))
	require.NoError(t, r.Confirm(id))
	entries, err = r.ReadAll()
	require.NoError(t, err)
	require.Empty(t, entries)
}

func clearSlotFrame(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	blank := make([]byte, RecoverySlotBytes)
	_, err = file.WriteAt(blank, 0)
	require.NoError(t, err)
	require.NoError(t, file.Sync())
}

// Confirm must never destroy evidence that landed after the caller last looked, while the
// replay loop still needs a way to release a slot whose events it has already consumed.
func TestClusterRecoveryReserveRefusesToConfirmNonEmptySlot(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 2)
	require.NoError(t, err)
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.NoError(t, r.SaveEvent(id, "revenue", []byte(`{"amount":2}`)))
	require.Error(t, r.Confirm(id), "a slot holding unreplayed evidence must refuse removal")
	entries, err := r.ReadAll()
	require.NoError(t, err)
	require.Contains(t, entries, id, "evidence must survive a confirm attempt")
	require.NotEmpty(t, entries[id])
	// The replay exit releases it: evidence is withdrawn first, then the slot is removed,
	// so a crash in between leaves an empty slot rather than lost evidence.
	require.NoError(t, r.ReleaseAfterReplay(id))
	entries, err = r.ReadAll()
	require.NoError(t, err)
	require.NotContains(t, entries, id)
}
func TestClusterRecoveryReserveKeepsPriorFrameAfterTornWrite(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 2)
	require.NoError(t, err)
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.NoError(t, r.Save(id, []byte(`{"amount":1}`)))
	require.NoError(t, r.Save(id, []byte(`{"amount":2}`)))
	file, err := os.OpenFile(filepath.Join(r.dir, id+".slot"), os.O_RDWR, 0600)
	require.NoError(t, err)
	_, err = file.WriteAt([]byte("broken"), recoveryFrameBytes+24)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	entries, err := r.ReadAll()
	require.NoError(t, err)
	require.JSONEq(t, `{"amount":1}`, string(entries[id]))
}
func TestClusterRecoveryReserveMultipleEvents(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 1)
	require.NoError(t, err)
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.NoError(t, r.SaveEvent(id, "a", []byte(`{"amount":1}`)))
	require.NoError(t, r.SaveEvent(id, "b", []byte(`{"amount":2}`)))
	require.NoError(t, r.SaveEvent(id, "a", []byte(`{"amount":1}`)))
	require.Error(t, r.SaveEvent(id, "a", []byte(`{"amount":3}`)))
	entries, err := r.ReadAll()
	require.NoError(t, err)
	var envelope RecoveryEvents
	require.NoError(t, json.Unmarshal(entries[id], &envelope))
	require.Len(t, envelope.Events, 2)
}

func TestClusterRecoveryReserveRejectsUnknownAndOversizedRecords(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 1)
	require.NoError(t, err)
	require.Error(t, r.Reserve("../escape"))
	require.Error(t, r.Save(uuid.NewString(), []byte(`{}`)))
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.Error(t, r.Save(id, []byte("not-json")))
}

func TestClusterRecoveryCompletionPreservesAllEvents(t *testing.T) {
	r, err := NewRecoveryReserve(t.TempDir(), 1)
	require.NoError(t, err)
	id := uuid.NewString()
	require.NoError(t, r.Reserve(id))
	require.NoError(t, r.SaveEvent(id, "a", []byte(`{"amount":1}`)))
	require.NoError(t, r.SaveCompletion(id, "needs_review"))
	require.NoError(t, r.SaveEvent(id, "b", []byte(`{"amount":2}`)))
	require.NoError(t, r.SaveCompletion(id, "awaiting_settlement"))
	entries, err := r.ReadAll()
	require.NoError(t, err)
	var envelope RecoveryEvents
	require.NoError(t, json.Unmarshal(entries[id], &envelope))
	require.Equal(t, "needs_review", envelope.FinishState)
	require.Len(t, envelope.Events, 2)
	require.Error(t, r.SaveCompletion(id, "settled"))
}
