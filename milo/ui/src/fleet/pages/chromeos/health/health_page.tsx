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
import { Box, Button, Typography } from '@mui/material';
import { useCallback, useMemo } from 'react';

import { useModelQuotaPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ExpectedQuotaCard } from './expected_quota_card';
import { HEALTH_FILTER_CONFIGS } from './filter_constants';
import { HeroAvailabilityCard } from './hero_availability_card';
import { HistoricalAvailabilityTrendsChart } from './historical_availability_trends_chart';
import { ManualQuotaOverridesCard } from './manual_quota_overrides_card';
import { SupportRiskIncidentsPanel } from './support_risk_incidents_panel';
import { useHealthFilters } from './use_health_filters';

export const HealthPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const { hasPermission: hasQuotaPermission } = useModelQuotaPermission();
  const canEditQuota = hasQuotaPermission === true;

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
                  gutterBottom
                  sx={{ fontWeight: 'bold' }}
                >
                  ChromeOS Fleet Health Metrics
                </Typography>
                <Typography variant="subtitle1" color="text.secondary">
                  Analyze hardware reliability trends, monitor model health, and
                  identify pool degradations.
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
            <Box sx={{ mb: 4 }}>
              <FilterBar
                filterCategoryDatas={filterCategoryDatas}
                isLoading={isLoading || filterValues === undefined}
                searchPlaceholder="Add a filter (e.g. model:volteer)"
              />
            </Box>

            {/* Hero Availability Card */}
            <HeroAvailabilityCard filter={activeFilter} />

            {/* Dashboard Layout */}
            <Box
              sx={{
                display: 'flex',
                flexDirection: { xs: 'column', lg: 'row' },
                gap: 4,
                alignItems: 'flex-start',
              }}
            >
              <Box sx={{ flex: 1, width: '100%', minWidth: 0 }}>
                <HistoricalAvailabilityTrendsChart filter={activeFilter} />
              </Box>
              <Box sx={{ width: { xs: '100%', lg: 440 }, flexShrink: 0 }}>
                <SupportRiskIncidentsPanel onShowModel={handleShowModel} />
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
