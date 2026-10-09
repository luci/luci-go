// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { ArrowBack, Settings } from '@mui/icons-material';
import { Box, Button, Tooltip, Typography } from '@mui/material';
import { useCallback, useMemo } from 'react';

import { useModelQuotaPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ExpectedQuotaCard } from './expected_quota_card';
import { HEALTH_FILTER_CONFIGS } from './filter_constants';
import { HealthFilterBar } from './health_filter_bar';
import { HealthSlicesCard } from './health_slices_card';
import { HistoricalAvailabilityTrendsChart } from './historical_availability_trends_chart';
import { ManualQuotaOverridesCard } from './manual_quota_overrides_card';
import { PerformanceRankingCard } from './performance_ranking_card';
import { SupportRiskIncidentsPanel } from './support_risk_incidents_panel';
import { useHealthFilters } from './use_health_filters';
import { useDefaultSliceSync } from './use_health_slices';

export const HealthPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const { hasPermission: hasQuotaPermission } = useModelQuotaPermission();
  const canEditQuota = hasQuotaPermission === true;

  useDefaultSliceSync();

  const rawTab = searchParams.get('tab');
  const pageTab: 'overview' | 'configuration' =
    rawTab === 'configuration' && canEditQuota ? 'configuration' : 'overview';

  const { filterValues, aip160, isLoading, warnings, setFiltersBatch } =
    useHealthFilters();
  const activeFilter = aip160();

  const filterCategoryDatas = useMemo(
    () => (filterValues ? Object.values(filterValues) : []),
    [filterValues],
  );

  const handleShowModel = useCallback(
    (model: string) => {
      setFiltersBatch({ [HEALTH_FILTER_CONFIGS.MODEL.key]: [model] });
    },
    [setFiltersBatch],
  );

  const handleShowPool = useCallback(
    (pool: string) => {
      setFiltersBatch({ [HEALTH_FILTER_CONFIGS.POOL.key]: [pool] });
    },
    [setFiltersBatch],
  );

  const handleSelectSlice = useCallback(
    (slice: HealthSlice) => {
      const sliceFilter = slice.filter.trim();
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        const currentFilter = next.get(FILTERS_PARAM_KEY) ?? '';
        if (sliceFilter && currentFilter === sliceFilter) {
          next.delete(FILTERS_PARAM_KEY);
        } else if (sliceFilter) {
          next.set(FILTERS_PARAM_KEY, sliceFilter);
        } else {
          next.delete(FILTERS_PARAM_KEY);
        }
        return next;
      });
    },
    [setSearchParams],
  );

  const setPageTab = (newTab: 'overview' | 'configuration') => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (newTab === 'overview') {
        next.delete('tab');
      } else {
        next.set('tab', newTab);
      }
      return next;
    });
  };

  return (
    <Box sx={{ p: 4, minHeight: '100vh' }}>
      <FleetHelmet
        pageTitle={`ChromeOS Fleet Health Metrics${
          pageTab === 'overview' ? '' : ' - Configuration'
        }`}
      />
      <Box sx={{ maxWidth: 1400, mx: 'auto' }}>
        {/* Initial Dashboard View: Title, Subtitle, Configuration Button, and Side Panel */}
        {pageTab === 'overview' && (
          <>
            <WarningNotifications warnings={warnings} />
            <Box
              sx={{
                mb: 4,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'flex-start',
                flexWrap: 'wrap',
                gap: 2,
              }}
            >
              <Box>
                <Typography
                  variant="h4"
                  component="h1"
                  sx={{ fontWeight: 'bold', mb: 0.5 }}
                >
                  ChromeOS Fleet Health Metrics
                </Typography>
                <Typography variant="subtitle1" color="text.secondary">
                  Analyze hardware reliability trends, monitor{' '}
                  <Tooltip
                    title={
                      <Box sx={{ p: 0.5 }}>
                        <Typography
                          variant="caption"
                          display="block"
                          sx={{ fontWeight: 'bold', mb: 0.5 }}
                        >
                          Availability vs. Health:
                        </Typography>
                        <Typography
                          variant="caption"
                          display="block"
                          sx={{ mb: 0.5 }}
                        >
                          • <b>Availability (Quota Baseline):</b> % of devices
                          available out of expected quota target configured per
                          model.
                        </Typography>
                        <Typography
                          variant="caption"
                          display="block"
                          sx={{ mb: 0.5 }}
                        >
                          • <b>Health (Enrolled Baseline):</b> Expected quotas
                          are configured per model overall. When filtered by
                          pool, zone, or arbitrary labels, baseline falls back
                          to actual enrolled physical DUTs.
                        </Typography>
                        <Typography
                          variant="caption"
                          display="block"
                          color="text.secondary"
                          sx={{ fontStyle: 'italic', mt: 0.5 }}
                        >
                          Note: Calculated based on devices in{' '}
                          <b>dut_state = READY</b>, which may differ from other
                          dashboards.
                        </Typography>
                      </Box>
                    }
                    arrow
                    slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
                  >
                    <Box
                      component="span"
                      data-testid="health-page-help-icon"
                      sx={{
                        borderBottom: '1px dashed',
                        borderColor: 'text.secondary',
                        cursor: 'help',
                        color: 'text.primary',
                        fontWeight: 500,
                        transition: 'all 0.15s ease-in-out',
                        '&:hover': {
                          color: 'primary.main',
                          borderColor: 'primary.main',
                        },
                      }}
                    >
                      availability and health
                    </Box>
                  </Tooltip>
                  , and identify pool degradations.
                </Typography>
              </Box>
              {canEditQuota && (
                <Box
                  sx={{
                    display: 'flex',
                    gap: 1.5,
                    alignItems: 'center',
                    flexWrap: 'wrap',
                  }}
                >
                  <Button
                    variant="outlined"
                    color="primary"
                    startIcon={<Settings />}
                    onClick={() => setPageTab('configuration')}
                    sx={{
                      textTransform: 'none',
                      fontWeight: 'bold',
                      px: 2,
                      py: 0.8,
                      borderRadius: 1.5,
                    }}
                  >
                    Configuration
                  </Button>
                </Box>
              )}
            </Box>

            {/* Global Filter Bar */}
            <Box sx={{ mb: 3 }}>
              <HealthFilterBar
                filterCategoryDatas={filterCategoryDatas}
                isLoading={isLoading || filterValues === undefined}
                searchPlaceholder="Add a filter (e.g. model:volteer)"
              />
            </Box>

            {/* Health Slices & Fleet Views Ribbon */}
            <HealthSlicesCard
              globalFilter={activeFilter}
              onSelectSlice={handleSelectSlice}
            />

            {/* Dashboard Layout */}
            <Box
              sx={{
                display: 'grid',
                gridTemplateColumns: { xs: '1fr', lg: '8fr 4fr' },
                gridTemplateRows: { xs: 'auto', lg: 'minmax(0, 1fr)' },
                height: { lg: 770 },
                gap: 4,
                mb: 4,
              }}
            >
              {/* Left Column: Interactive Trend Chart & Support Risk Incidents */}
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 4,
                  height: '100%',
                  minHeight: 0,
                }}
              >
                <HistoricalAvailabilityTrendsChart filter={activeFilter} />
                <Box
                  sx={{
                    flexGrow: 1,
                    minHeight: { xs: 260, lg: 0 },
                  }}
                >
                  <SupportRiskIncidentsPanel onShowModel={handleShowModel} />
                </Box>
              </Box>

              {/* Right Column: Performance Ranking Card */}
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 4,
                  height: '100%',
                  minHeight: 0,
                }}
              >
                <PerformanceRankingCard
                  filter={activeFilter}
                  onSelectModel={handleShowModel}
                  onSelectPool={handleShowPool}
                />
              </Box>
            </Box>
          </>
        )}

        {/* Configuration View: Back button, Header, and Expected Quota Card */}
        {pageTab === 'configuration' && (
          <Box sx={{ mb: 4 }}>
            <Button
              startIcon={<ArrowBack />}
              onClick={() => setPageTab('overview')}
              sx={{
                textTransform: 'none',
                fontWeight: 'bold',
                mb: 2,
                color: 'text.secondary',
                '&:hover': { color: 'text.primary' },
              }}
            >
              Back to Fleet Overview
            </Button>
            <Box sx={{ mb: 4 }}>
              <Typography
                variant="h4"
                component="h1"
                gutterBottom
                sx={{ fontWeight: 'bold' }}
              >
                Fleet Health Configuration
              </Typography>
              <Typography variant="subtitle1" color="text.secondary">
                Configure baseline hardware expected fleet quantities.
              </Typography>
            </Box>

            <Box
              sx={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 4,
                alignItems: 'flex-start',
              }}
            >
              <ExpectedQuotaCard />
              <ManualQuotaOverridesCard />
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  );
};

export const Component = HealthPage;
export default HealthPage;
