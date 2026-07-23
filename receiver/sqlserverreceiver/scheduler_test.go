// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package sqlserverreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sqlserverreceiver/internal/metadata"
)

func TestValidateCollectionGroups(t *testing.T) {
	receiverInterval := 10 * time.Second

	testCases := []struct {
		name        string
		groupName   string
		interval    time.Duration
		expectError string
	}{
		{
			name:      "zero inherits receiver interval",
			groupName: "index_physical",
			interval:  0,
		},
		{
			name:      "equal to receiver interval",
			groupName: "index_physical",
			interval:  receiverInterval,
		},
		{
			name:      "greater than receiver interval",
			groupName: "index_physical",
			interval:  30 * time.Minute,
		},
		{
			name:        "less than receiver interval",
			groupName:   "index_physical",
			interval:    5 * time.Second,
			expectError: "`collection_groups.index_physical.collection_interval` must be greater than or equal to the receiver `collection_interval`",
		},
		{
			name:        "negative interval",
			groupName:   "wait_stats",
			interval:    -1 * time.Second,
			expectError: "`collection_groups.wait_stats.collection_interval` must not be less than 0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCollectionGroup(tc.groupName, tc.interval, receiverInterval)
			if tc.expectError == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectError)
		})
	}
}

func TestConfigValidateCollectionGroups(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second
	cfg.CollectionGroups.IndexPhysical.CollectionInterval = 30 * time.Minute
	assert.NoError(t, cfg.Validate())

	cfg.CollectionGroups.IndexPhysical.CollectionInterval = 5 * time.Second
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection_groups.index_physical.collection_interval")
}

func TestEffectiveCollectionInterval(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second

	assert.Equal(t, 10*time.Second, cfg.effectiveCollectionInterval(CollectionGroupConfig{}))
	assert.Equal(t, 30*time.Minute, cfg.effectiveCollectionInterval(CollectionGroupConfig{
		CollectionInterval: 30 * time.Minute,
	}))
}

func TestShouldCollect(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second

	scraper := &sqlServerScraperHelper{
		config:             cfg,
		queryGroup:         queryGroupIndexPhysical,
		collectionInterval: 30 * time.Minute,
	}

	now := time.Now()
	assert.True(t, scraper.shouldCollect(now))

	scraper.lastCollectionTime = now
	assert.False(t, scraper.shouldCollect(now.Add(5*time.Minute)))
	assert.True(t, scraper.shouldCollect(now.Add(30*time.Minute)))

	scraper.collectionInterval = cfg.ControllerConfig.CollectionInterval
	assert.True(t, scraper.shouldCollect(now))

	scraper.collectionInterval = 0
	assert.True(t, scraper.shouldCollect(now))
}

func TestSetupQuerySpecsUsesCollectionGroupIntervals(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second
	configureAllScraperMetricsAndEvents(cfg, false)
	cfg.Metrics.SqlserverIndexFragmentation.Enabled = true
	cfg.Metrics.SqlserverCPUCount.Enabled = true
	cfg.CollectionGroups.IndexPhysical.CollectionInterval = 30 * time.Minute
	cfg.CollectionGroups.ServerProperties.CollectionInterval = 5 * time.Minute

	specs := setupQuerySpecs(cfg)
	require.Len(t, specs, 2)

	specByName := make(map[queryGroupName]querySpec, len(specs))
	for _, spec := range specs {
		specByName[spec.name] = spec
	}

	assert.Equal(t, 30*time.Minute, specByName[queryGroupIndexPhysical].collectionInterval)
	assert.Equal(t, 5*time.Minute, specByName[queryGroupServerProperties].collectionInterval)
}

func TestScrapeMetricsSkipsUntilDue(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second
	cfg.Server = "0.0.0.0"
	cfg.Username = "sa"
	cfg.Password = "password"
	cfg.Port = 1433
	configureAllScraperMetricsAndEvents(cfg, false)
	cfg.Metrics.SqlserverIndexFragmentation.Enabled = true
	cfg.CollectionGroups.IndexPhysical.CollectionInterval = time.Hour
	require.NoError(t, cfg.Validate())

	specs := setupQuerySpecs(cfg)
	require.Len(t, specs, 1)

	settings := receivertest.NewNopSettings(metadata.Type)
	scrapers := setupSQLServerScrapers(settings, cfg)
	require.Len(t, scrapers, 1)
	scraper := scrapers[0]
	scraper.client = mockClient{
		instanceName: scraper.config.InstanceName,
		SQL:          scraper.sqlQuery,
	}

	firstMetrics, err := scraper.ScrapeMetrics(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, firstMetrics.ResourceMetrics().Len())

	secondMetrics, err := scraper.ScrapeMetrics(t.Context())
	require.NoError(t, err)
	assert.Empty(t, secondMetrics.ResourceMetrics().Len())
}

func TestIsServerPropertiesQueryEnabled(t *testing.T) {
	assert.False(t, isServerPropertiesQueryEnabled(nil))

	metrics := &metadata.MetricsConfig{}
	assert.False(t, isServerPropertiesQueryEnabled(metrics))

	metrics.SqlserverCPUCount.Enabled = true
	assert.True(t, isServerPropertiesQueryEnabled(metrics))

	metrics.SqlserverCPUCount.Enabled = false
	metrics.SqlserverDatabaseCount.Enabled = true
	assert.True(t, isServerPropertiesQueryEnabled(metrics))

	metrics.SqlserverDatabaseCount.Enabled = false
	metrics.SqlserverComputerUptime.Enabled = true
	assert.True(t, isServerPropertiesQueryEnabled(metrics))
}

func TestQueryRegistryMapsAllGroups(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	configureAllScraperMetricsAndEvents(cfg, true)
	cfg.Metrics.SqlserverCPUCount.Enabled = true
	cfg.Metrics.SqlserverComputerUptime.Enabled = true
	cfg.Metrics.SqlserverAvailabilityGroupDatabaseReplicaSecondaryLag.Enabled = true

	specs := setupQuerySpecs(cfg)
	require.Len(t, specs, len(metricQueryDefinitions))

	got := make(map[queryGroupName]struct{}, len(specs))
	for _, spec := range specs {
		got[spec.name] = struct{}{}
		assert.NotEmpty(t, spec.query)
		assert.NotNil(t, spec.recordMetrics)
		assert.Equal(t, cfg.ControllerConfig.CollectionInterval, spec.collectionInterval)
	}

	for _, def := range metricQueryDefinitions {
		_, ok := got[def.name]
		assert.True(t, ok, "missing query group %s", def.name)
	}
}

func TestScrapeMetricsDoesNotAdvanceDeadlineOnError(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.ControllerConfig.CollectionInterval = 10 * time.Second
	cfg.Server = "0.0.0.0"
	cfg.Username = "sa"
	cfg.Password = "password"
	cfg.Port = 1433
	cfg.Metrics.SqlserverIndexFragmentation.Enabled = true
	cfg.CollectionGroups.IndexPhysical.CollectionInterval = time.Hour
	require.NoError(t, cfg.Validate())

	scraper := &sqlServerScraperHelper{
		config:             cfg,
		queryGroup:         queryGroupIndexPhysical,
		collectionInterval: time.Hour,
		recordMetrics: func(_ *sqlServerScraperHelper, _ context.Context) error {
			return assert.AnError
		},
	}

	_, err := scraper.ScrapeMetrics(t.Context())
	require.Error(t, err)
	assert.True(t, scraper.lastCollectionTime.IsZero())
}

func TestSetupQueriesReturnsQueriesFromRegistry(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	configureAllScraperMetricsAndEvents(cfg, false)
	cfg.Metrics.SqlserverWorkerThreadCount.Enabled = true

	queries := setupQueries(cfg)
	specs := setupQuerySpecs(cfg)
	require.Len(t, queries, 1)
	require.Len(t, specs, 1)
	assert.Equal(t, getSQLServerWorkerThreadsQuery(cfg.InstanceName), queries[0])
	assert.Equal(t, queryGroupWorkerThreads, specs[0].name)
}

func TestCollectionGroupsConfigDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	assert.Equal(t, time.Duration(0), cfg.CollectionGroups.IndexPhysical.CollectionInterval)
	assert.Equal(t, 10*time.Second, cfg.ControllerConfig.CollectionInterval)
}

func TestScraperIDUsesQueryGroupName(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Server = "0.0.0.0"
	cfg.Username = "sa"
	cfg.Password = "password"
	cfg.Port = 1433
	configureAllScraperMetricsAndEvents(cfg, false)
	cfg.Metrics.SqlserverWorkerThreadCount.Enabled = true
	require.NoError(t, cfg.Validate())

	scrapers := setupSQLServerScrapers(receivertest.NewNopSettings(metadata.Type), cfg)
	require.Len(t, scrapers, 1)
	assert.Equal(t, component.NewIDWithName(metadata.Type, "query-worker_threads"), scrapers[0].id)
}
