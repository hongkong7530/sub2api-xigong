package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJailbreakRefreshFailureSharedAndColdReadReturnsError(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	unavailable := errors.New("settings unavailable")
	RegisterJailbreakTemplateRefresher(func(context.Context) error {
		calls.Add(1)
		close(entered)
		<-release
		return unavailable
	})
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			_, err := (&APIKeyService{}).ListAvailableJailbreakTemplates(context.Background())
			results <- err
		}()
	}
	<-entered
	close(release)
	for i := 0; i < 8; i++ {
		require.ErrorIs(t, <-results, unavailable)
	}
	require.EqualValues(t, 1, calls.Load(), "failed reads must share one result during cooldown")
}

func TestJailbreakRefreshWaitCanBeCancelled(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	entered, release := make(chan struct{}), make(chan struct{})
	RegisterJailbreakTemplateRefresher(func(context.Context) error {
		close(entered)
		<-release
		applyManagedJailbreakTemplates([]JailbreakTemplateRecord{{Name: "loaded", Enabled: true, SystemPrompt: "content"}})
		return nil
	})
	finished := make(chan error, 1)
	go func() { finished <- maybeRefreshManagedJailbreakTemplates(context.Background()) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, maybeRefreshManagedJailbreakTemplates(ctx), context.Canceled)
	close(release)
	require.NoError(t, <-finished)
}

func TestJailbreakWarmSnapshotDoesNotWaitForFailedRefresh(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	applyManagedJailbreakTemplates([]JailbreakTemplateRecord{{Name: "warm", Enabled: true, SystemPrompt: "content"}})
	managedJailbreakMu.Lock()
	managedJailbreakRefreshed = time.Now().Add(-time.Minute)
	managedJailbreakMu.Unlock()
	release := make(chan struct{})
	RegisterJailbreakTemplateRefresher(func(context.Context) error { <-release; return errors.New("database unavailable") })
	tpl, err := LoadJailbreakTemplate("warm")
	require.NoError(t, err)
	require.Equal(t, "content", tpl.SystemPrompt)
	managedJailbreakMu.RLock()
	done := managedJailbreakInFlight
	managedJailbreakMu.RUnlock()
	require.NotNil(t, done)
	close(release)
	<-done
	tpl, err = LoadJailbreakTemplate("warm")
	require.NoError(t, err)
	require.Equal(t, "content", tpl.SystemPrompt)
}

type jailbreakSnapshotRepo struct {
	SettingRepository
	entered, release chan struct{}
	value            string
}

func (r *jailbreakSnapshotRepo) GetValue(context.Context, string) (string, error) {
	snapshot := r.value
	close(r.entered)
	<-r.release
	return snapshot, nil
}
func (r *jailbreakSnapshotRepo) Set(_ context.Context, _ string, value string) error {
	r.value = value
	return nil
}

func TestJailbreakRefreshCannotOverwriteNewSave(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	repo := &jailbreakSnapshotRepo{entered: make(chan struct{}), release: make(chan struct{}), value: `[{"name":"race","enabled":false,"system_prompt":"old"}]`}
	settings := NewSettingService(repo, nil)
	finished := make(chan error, 1)
	go func() { finished <- settings.RefreshManagedJailbreakTemplates(context.Background()) }()
	<-repo.entered
	require.NoError(t, settings.SaveJailbreakTemplateRecords(context.Background(), []JailbreakTemplateRecord{{Name: "race", Enabled: true, SystemPrompt: "new"}}))
	close(repo.release)
	require.NoError(t, <-finished)
	tpl, ok := managedJailbreakTemplateContent("race")
	require.True(t, ok)
	require.Equal(t, "new", tpl.SystemPrompt)
	require.True(t, managedJailbreakTemplateRecords()[0].Enabled)
}
