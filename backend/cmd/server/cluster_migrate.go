package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// This entry point deliberately does not instantiate the server's Wire graph.
func runClusterMigrations(cfg *config.Config) error {
	if !cfg.ClusterEnabled() || !cfg.RunsRole(config.RoleMigrator) {
		return fmt.Errorf("standalone migrator role required")
	}
	client, _, err := repository.InitEnt(cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	settings := service.NewSettingService(repository.NewSettingRepository(client), cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, migrate := range []func(context.Context) error{
		settings.MigrateOpenAIAllowClaudeCodeCodexPluginSetting,
		settings.MigrateCodexBodyFingerprintToSignals,
		settings.MigrateGrokDefaultTextModel,
	} {
		if err := migrate(ctx); err != nil {
			return err
		}
	}
	return nil
}
