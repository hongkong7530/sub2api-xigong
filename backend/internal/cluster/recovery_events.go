package cluster

import (
	"encoding/json"
	"fmt"
	"os"
)

type RecoveryEvents struct {
	FinishState string                     `json:"finish_state,omitempty"`
	Version     int                        `json:"version"`
	Events      map[string]json.RawMessage `json:"events"`
}

// SaveEvent stores all unconfirmed monetary effects in one preallocated execution slot.
// The payload is bounded; repeated evidence for the same event must remain identical.
func (r *RecoveryReserve) SaveEvent(id, event string, data []byte) error {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	path, err := r.slot(id)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	previous, _, _ := decodeRecoverySlot(raw)
	envelope := RecoveryEvents{Version: 1, Events: map[string]json.RawMessage{}}
	if len(previous) > 0 {
		if json.Unmarshal(previous, &envelope) != nil || envelope.Version != 1 || envelope.Events == nil {
			return fmt.Errorf("invalid recovery event envelope")
		}
	}
	if existing, ok := envelope.Events[event]; ok {
		var oldValue, newValue any
		if json.Unmarshal(existing, &oldValue) != nil || json.Unmarshal(data, &newValue) != nil {
			return fmt.Errorf("invalid event evidence")
		}
		oldJSON, _ := json.Marshal(oldValue)
		newJSON, _ := json.Marshal(newValue)
		if string(oldJSON) != string(newJSON) {
			return fmt.Errorf("conflicting recovery event evidence")
		}
		return nil
	}
	if len(envelope.Events) >= 16 {
		return fmt.Errorf("recovery event count exceeds reserved execution budget")
	}
	envelope.Events[event] = append(json.RawMessage(nil), data...)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	// Leave room for the completion marker even when monetary evidence fills the slot.
	if len(encoded) > recoveryFrameBytes-24-256 {
		return fmt.Errorf("recovery event evidence exceeds reserved budget")
	}
	return r.Save(id, encoded)
}

// SaveCompletion preserves a failed database completion write without inventing a money event.
func (r *RecoveryReserve) SaveCompletion(id, state string) error {
	if !validCompletionState(state) {
		return fmt.Errorf("invalid execution completion state")
	}
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	path, err := r.slot(id)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	previous, _, _ := decodeRecoverySlot(raw)
	envelope := RecoveryEvents{Version: 1, Events: map[string]json.RawMessage{}}
	if len(previous) > 0 {
		if json.Unmarshal(previous, &envelope) != nil || envelope.Version != 1 || envelope.Events == nil {
			return fmt.Errorf("invalid recovery event envelope")
		}
	}
	// A later generic request finalizer must not erase the more conservative turn result.
	if envelope.FinishState != "" {
		return nil
	}
	envelope.FinishState = state
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return r.Save(id, encoded)
}
