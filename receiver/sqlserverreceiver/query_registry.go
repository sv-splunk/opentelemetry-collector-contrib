// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package sqlserverreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sqlserverreceiver"

import (
	"context"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sqlserverreceiver/internal/metadata"
)

type queryGroupName string

const (
	queryGroupAvailabilityGroup   queryGroupName = "availability_group"
	queryGroupDatabaseIO          queryGroupName = "database_io"
	queryGroupPerformanceCounters queryGroupName = "performance_counters"
	queryGroupServerProperties    queryGroupName = "server_properties"
	queryGroupWaitStats           queryGroupName = "wait_stats"
	queryGroupWorkerThreads       queryGroupName = "worker_threads"
	queryGroupIndexPhysical       queryGroupName = "index_physical"
)

type querySpec struct {
	name               queryGroupName
	query              string
	collectionInterval time.Duration
	recordMetrics      func(*sqlServerScraperHelper, context.Context) error
}

type queryDefinition struct {
	name               queryGroupName
	buildQuery         func(instanceName string) string
	isRequired         func(*metadata.MetricsConfig) bool
	configuredInterval func(*Config) time.Duration
	recordMetrics      func(*sqlServerScraperHelper, context.Context) error
}

var metricQueryDefinitions = []queryDefinition{
	{
		name:               queryGroupAvailabilityGroup,
		buildQuery:         getSQLServerAvailabilityGroupQuery,
		isRequired:         isAvailabilityGroupQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.AvailabilityGroup.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordAvailabilityGroupMetrics,
	},
	{
		name:               queryGroupDatabaseIO,
		buildQuery:         getSQLServerDatabaseIOQuery,
		isRequired:         isDatabaseIOQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.DatabaseIO.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordDatabaseIOMetrics,
	},
	{
		name:               queryGroupPerformanceCounters,
		buildQuery:         getSQLServerPerformanceCounterQuery,
		isRequired:         isPerfCounterQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.PerformanceCounters.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordDatabasePerfCounterMetrics,
	},
	{
		name:               queryGroupServerProperties,
		buildQuery:         getSQLServerPropertiesQuery,
		isRequired:         isServerPropertiesQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.ServerProperties.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordDatabaseStatusMetrics,
	},
	{
		name:               queryGroupWaitStats,
		buildQuery:         getSQLServerWaitStatsQuery,
		isRequired:         isWaitStatsQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.WaitStats.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordDatabaseWaitMetrics,
	},
	{
		name:               queryGroupWorkerThreads,
		buildQuery:         getSQLServerWorkerThreadsQuery,
		isRequired:         isWorkerThreadsQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.WorkerThreads.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordWorkerThreadMetrics,
	},
	{
		name:               queryGroupIndexPhysical,
		buildQuery:         getSQLServerIndexPhysicalStatsQuery,
		isRequired:         isIndexPhysicalStatsQueryEnabled,
		configuredInterval: func(cfg *Config) time.Duration { return cfg.CollectionGroups.IndexPhysical.CollectionInterval },
		recordMetrics:      (*sqlServerScraperHelper).recordIndexPhysicalMetrics,
	},
}

func setupQuerySpecs(cfg *Config) []querySpec {
	var specs []querySpec

	for _, def := range metricQueryDefinitions {
		if !def.isRequired(&cfg.Metrics) {
			continue
		}

		specs = append(specs, querySpec{
			name:               def.name,
			query:              def.buildQuery(cfg.InstanceName),
			collectionInterval: cfg.effectiveCollectionInterval(CollectionGroupConfig{CollectionInterval: def.configuredInterval(cfg)}),
			recordMetrics:      def.recordMetrics,
		})
	}

	return specs
}

func isServerPropertiesQueryEnabled(metrics *metadata.MetricsConfig) bool {
	if metrics == nil {
		return false
	}

	return metrics.SqlserverDatabaseCount.Enabled ||
		metrics.SqlserverCPUCount.Enabled ||
		metrics.SqlserverComputerUptime.Enabled
}
