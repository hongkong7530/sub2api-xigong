package repository

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration271AcceptsOnlyVerifiedRawAndTrimmedHashes(t *testing.T) {
	const name = "271_fix_restored_table_ownership.sql"
	raw, err := migrations.FS.ReadFile(name)
	require.NoError(t, err)
	original := fmt.Sprintf("%x", sha256.Sum256(raw))
	trimmed := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(string(raw)))))
	require.Equal(t, "956a3c4cce6680b3ba28b21b31fd2a51d5fbd6df19172b6d9decace52b1bd2b3", original)
	require.Equal(t, "55a7722fe08e55867261220aeac92e6d6a4b3dca057900962049c3ad29ef40ac", trimmed)
	require.True(t, isMigrationChecksumCompatible(name, original, trimmed))
	require.False(t, isMigrationChecksumCompatible(name, "unverified", trimmed))
	require.False(t, isMigrationChecksumCompatible(name, original, "modified-file"))
	require.False(t, isMigrationChecksumCompatible("272_restore_recorded_recharge_totals.sql", original, trimmed))
}
