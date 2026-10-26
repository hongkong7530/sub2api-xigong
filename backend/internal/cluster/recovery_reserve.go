package cluster

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const RecoverySlotBytes = 128 * 1024
const recoveryFrameBytes = RecoverySlotBytes / 2

// RecoveryReserve owns fixed disk slots allocated before dispatch. Two checksummed
// frames preserve the last valid result if a process dies during a replacement.
type RecoveryReserve struct {
	eventMu sync.Mutex
	mu      sync.Mutex
	dir     string
	max     int
}

func NewRecoveryReserve(stateDir string, max int) (*RecoveryReserve, error) {
	dir := filepath.Join(stateDir, "recovery-reserve")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe recovery reserve directory")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	return &RecoveryReserve{dir: dir, max: max}, nil
}
func (r *RecoveryReserve) slot(id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("recovery slot requires an execution UUID")
	}
	return filepath.Join(r.dir, id+".slot"), nil
}
// ConfigureCapacity follows the registry without deleting existing obligations when
// capacity shrinks. A full or unreadable reserve must stop new admission.
func (r *RecoveryReserve) ConfigureCapacity(max int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if max < 1 {
		return false, fmt.Errorf("invalid recovery reserve capacity")
	}
	r.max = max
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return false, err
	}
	return len(entries) < r.max, nil
}

func (r *RecoveryReserve) Reserve(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path, err := r.slot(id)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return err
	}
	if len(entries) >= r.max {
		return fmt.Errorf("recovery capacity exhausted")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	// Explicit writes allocate real blocks; Truncate alone may create a sparse file.
	block := make([]byte, 8192)
	for written := 0; written < RecoverySlotBytes; written += len(block) {
		if _, err = f.Write(block); err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	ok = true
	return syncRecoveryDirectory(r.dir)
}
func syncRecoveryDirectory(path string) error {
	if os.PathSeparator == '\\' {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (r *RecoveryReserve) Save(id string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(data) == 0 || len(data) > recoveryFrameBytes-24 || !json.Valid(data) {
		return fmt.Errorf("invalid recovery evidence size or JSON")
	}
	path, err := r.slot(id)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Size() != RecoverySlotBytes {
		return fmt.Errorf("invalid recovery slot")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, RecoverySlotBytes))
	if err != nil {
		return err
	}
	_, generation, frame := decodeRecoverySlot(raw)
	next := 0
	if frame == 0 {
		next = 1
	}
	buffer := make([]byte, recoveryFrameBytes)
	copy(buffer[:8], []byte("S2BILL01"))
	binary.LittleEndian.PutUint64(buffer[8:16], generation+1)
	binary.LittleEndian.PutUint32(buffer[16:20], uint32(len(data)))
	binary.LittleEndian.PutUint32(buffer[20:24], crc32.ChecksumIEEE(data))
	copy(buffer[24:], data)
	if _, err = f.WriteAt(buffer, int64(next*recoveryFrameBytes)); err != nil {
		return err
	}
	return f.Sync()
}
func decodeRecoverySlot(raw []byte) ([]byte, uint64, int) {
	var best []byte
	var generation uint64
	frame := -1
	for index := 0; index < 2; index++ {
		offset := index * recoveryFrameBytes
		if len(raw) < offset+recoveryFrameBytes {
			continue
		}
		part := raw[offset : offset+recoveryFrameBytes]
		if string(part[:8]) != "S2BILL01" {
			continue
		}
		n := int(binary.LittleEndian.Uint32(part[16:20]))
		if n < 1 || n > recoveryFrameBytes-24 {
			continue
		}
		data := part[24 : 24+n]
		if crc32.ChecksumIEEE(data) != binary.LittleEndian.Uint32(part[20:24]) || !json.Valid(data) {
			continue
		}
		g := binary.LittleEndian.Uint64(part[8:16])
		if g > generation {
			best = append([]byte(nil), data...)
			generation = g
			frame = index
		}
	}
	return best, generation, frame
}
func (r *RecoveryReserve) ReadAll() (map[string][]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	result := map[string][]byte{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".slot") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe recovery entry")
		}
		path := filepath.Join(r.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() != RecoverySlotBytes {
			return nil, fmt.Errorf("invalid recovery slot size")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		data, _, _ := decodeRecoverySlot(raw)
		result[strings.TrimSuffix(entry.Name(), ".slot")] = data
	}
	return result, nil
}
func (r *RecoveryReserve) Confirm(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("recovery slot requires an execution UUID")
	}
	path := filepath.Join(r.dir, id+".slot")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe recovery entry")
	}
	// The slot is re-read under the same lock that serialises Save: a caller's earlier
	// "slot is empty" reading is not evidence, because evidence may have landed since.
	if raw, err := os.ReadFile(path); err == nil {
		if evidence, _, _ := decodeRecoverySlot(raw); len(evidence) > 0 {
			return fmt.Errorf("recovery slot %s still holds unreplayed evidence; refusing to remove it", id)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.removeLocked(path)
}

// ReleaseAfterReplay removes a slot whose events have already been consumed, recording the
// fact before the delete so a crash cannot lose evidence. Confirm deliberately refuses a
// non-empty slot, because a caller's earlier reading is only a snapshot; the replay loop is
// the one place that legitimately knows the events are spent, and it goes through here.
func (r *RecoveryReserve) ReleaseAfterReplay(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("recovery slot requires an execution UUID")
	}
	path := filepath.Join(r.dir, id+".slot")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe recovery entry")
	}
	// Order matters: withdraw the evidence first, then delete. A crash in between leaves an
	// empty slot, which the next replay pass deletes cheaply; the reverse order could drop
	// evidence that was never consumed.
	if raw, err := os.ReadFile(path); err == nil {
		if _, _, frame := decodeRecoverySlot(raw); frame >= 0 {
			file, openErr := os.OpenFile(path, os.O_RDWR, 0600)
			if openErr != nil {
				return openErr
			}
			blank := make([]byte, RecoverySlotBytes)
			_, writeErr := file.WriteAt(blank, 0)
			if writeErr == nil {
				writeErr = file.Sync()
			}
			closeErr := file.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.removeLocked(path)
}

func (r *RecoveryReserve) removeLocked(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncRecoveryDirectory(r.dir)
}
