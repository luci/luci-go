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

import CheckIcon from '@mui/icons-material/Check';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Box, Grid, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { SmallMetricItem } from '@/fleet/components/summary_header/small_metric_item';
import {
  SubMetricList,
  SummaryHeaderTitle,
} from '@/fleet/components/summary_header/summary_header_layout';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { getMetricsGridStyles } from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useBrowserFilters } from './use_browser_filters';

export function BrowserSummaryHeader() {
  const client = useFleetConsoleClient();
  const theme = useTheme();
  const { filterValues, aip160 } = useBrowserFilters();

  const gridStyles = getMetricsGridStyles(theme);

  // Consolidated multi-dimensional count query grouping by UFS resource state and bot health status.
  const countQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: aip160(),
      groupBy: [`${BROWSER_UFS_SOURCE}.resource_state`, 'bot_health_status'],
    }),
  );

  const setFilterOptions = (key: string, options: string[]) => {
    const filter = filterValues?.[key];
    if (filter instanceof StringListFilterCategory) {
      filter.setSelectedOptions(options);
    }
  };

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
      );
    }

    const isLoading = countQuery.isPending || !countQuery.data;

    const totalDevices = countQuery.data?.total ?? 0;
    const records = countQuery.data?.records ?? [];

    let swTotal = 0;
    let healthyCount = 0;
    let sDead = 0;
    let sQuarantined = 0;
    let sMaintenance = 0;
    let sMissingBots = 0;
    let ufsNeedsRepair = 0;
    let ufsMissing = 0;
    let ufsExcluded = 0;
    let nonServingOthers = 0;

    const EXCLUDED_UFS_STATES = new Set([
      'RESERVED',
      'DECOMMISSIONED',
      'DEPLOYED_TESTING',
      'DEPLOYING',
      'REGISTERED',
    ]);

    if (records.length > 0) {
      for (const record of records) {
        const cnt = Number(record.count) || 0;
        const ufsState =
          record.dimensions[`${BROWSER_UFS_SOURCE}.resource_state`] ||
          record.dimensions['ufs_labels.resource_state'] ||
          record.dimensions['state'] ||
          '';
        const botHealth = record.dimensions['bot_health_status'] || '';

        if (botHealth !== 'MISSING' && botHealth !== '') {
          swTotal += cnt;
        }

        if (ufsState === 'SERVING') {
          switch (botHealth) {
            case 'HEALTHY':
              healthyCount += cnt;
              break;
            case 'DEAD':
              sDead += cnt;
              break;
            case 'QUARANTINED':
              sQuarantined += cnt;
              break;
            case 'MAINTENANCE':
              sMaintenance += cnt;
              break;
            case 'MISSING':
            default:
              sMissingBots += cnt;
              break;
          }
        } else if (ufsState === 'NEEDS_REPAIR') {
          ufsNeedsRepair += cnt;
        } else if (ufsState === 'MISSING') {
          ufsMissing += cnt;
        } else if (EXCLUDED_UFS_STATES.has(ufsState)) {
          ufsExcluded += cnt;
        } else {
          nonServingOthers += cnt;
        }
      }
    } else if (countQuery.data?.swarmingState) {
      // Fallback for flat count responses when group_by records are not present.
      swTotal = countQuery.data.swarmingState.total ?? 0;
      const sAlive = countQuery.data.swarmingState.alive ?? 0;
      sDead = countQuery.data.swarmingState.dead ?? 0;
      sQuarantined = countQuery.data.swarmingState.quarantined ?? 0;
      sMaintenance = countQuery.data.swarmingState.maintenance ?? 0;
      healthyCount = Math.max(0, sAlive - sQuarantined - sMaintenance);
    }

    // Unhealthy (Broken): expected serving but failed + UFS states indicating issues (NEEDS_REPAIR, MISSING)
    const unhealthyCount =
      sDead + sQuarantined + sMaintenance + ufsNeedsRepair + ufsMissing;
    // Other (Reserved / Excluded): Excluded UFS states + serving devices missing bots + unspecified remaining
    const otherCount = ufsExcluded + nonServingOthers + sMissingBots;

    const handleHealthyClick = () => {
      setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, ['alive']);
      setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, ['SERVING']);
    };

    const handleUnhealthyHeaderClick = () => {
      setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [
        'dead',
        'quarantined',
        'maintenance',
        BLANK_VALUE,
      ]);
      setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, [
        'SERVING',
        'NEEDS_REPAIR',
        'MISSING',
      ]);
    };

    const handleSwarmingIssuesClick = () => {
      setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [
        'dead',
        'quarantined',
        'maintenance',
      ]);
      setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, ['SERVING']);
    };

    const handleUfsIssuesClick = () => {
      setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, []);
      setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, [
        'NEEDS_REPAIR',
        'MISSING',
      ]);
    };

    const handleUnhealthyClick = (state: string) => {
      setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [state]);
      setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, ['SERVING']);
    };

    const handleNonServingClick = () => {
      const filter = filterValues?.[`${BROWSER_UFS_SOURCE}."resource_state"`];
      if (filter instanceof StringListFilterCategory) {
        const nonServingOptions = Object.keys(filter.getOptions()).filter(
          (val) => val !== 'SERVING' && val !== '',
        );
        filter.setSelectedOptions(nonServingOptions);
      }
    };

    const handleClearFilters = () => {
      if (filterValues) {
        Object.values(filterValues).forEach((filter) => {
          if (filter instanceof StringListFilterCategory) {
            filter.setSelectedOptions([]);
          }
        });
      }
    };

    return (
      <Box sx={{ p: 1 }}>
        <Grid container spacing={0} alignItems="stretch">
          {/* Column 1: Total Devices */}
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col1}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Total Devices"
                value={isLoading ? undefined : totalDevices}
                loading={isLoading}
                handleClick={handleClearFilters}
              />
            </Box>

            <SubMetricList>
              <SmallMetricItem
                label="With Bots:"
                value={swTotal}
                total={totalDevices}
                dotColor={theme.palette.success.main}
                loading={isLoading}
              />
            </SubMetricList>
          </Grid>

          {/* Column 2: Healthy Devices */}
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col2}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Healthy"
                value={isLoading ? undefined : healthyCount}
                total={totalDevices}
                loading={isLoading}
                Icon={<CheckIcon sx={{ color: theme.palette.success.main }} />}
                handleClick={handleHealthyClick}
              />
              <InfoTooltip>
                <Typography variant="body2">
                  Operational test runners. Device must be SERVING in UFS and
                  Alive in Swarming (excluding quarantined or maintenance bots).
                </Typography>
              </InfoTooltip>
            </Box>

            <SubMetricList>
              <SmallMetricItem
                label="Alive & Serving:"
                value={healthyCount}
                total={totalDevices}
                onClick={handleHealthyClick}
                dotColor={theme.palette.success.main}
                loading={isLoading}
              />
            </SubMetricList>
          </Grid>

          {/* Column 3: Unhealthy Devices */}
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col3}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Unhealthy"
                value={isLoading ? undefined : unhealthyCount}
                total={totalDevices}
                loading={isLoading}
                Icon={<ErrorIcon sx={{ color: theme.palette.error.main }} />}
                handleClick={handleUnhealthyHeaderClick}
              />
              <InfoTooltip>
                <Typography variant="body2">
                  Devices currently broken or out of service. Includes dead or
                  quarantined bots on serving devices, plus devices in
                  NeedsRepair or Maintenance states in UFS.
                </Typography>
              </InfoTooltip>
            </Box>

            <SubMetricList>
              <SmallMetricItem
                label="Swarming Bot Issues:"
                value={sDead + sQuarantined + sMaintenance}
                total={totalDevices}
                onClick={handleSwarmingIssuesClick}
                dotColor={theme.palette.error.main}
                loading={isLoading}
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  label="Offline / Dead:"
                  value={sDead}
                  total={totalDevices}
                  onClick={() => handleUnhealthyClick('dead')}
                  dotColor={theme.palette.error.main}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Quarantined:"
                  value={sQuarantined}
                  total={totalDevices}
                  onClick={() => handleUnhealthyClick('quarantined')}
                  dotColor={theme.palette.warning.main}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="In Maintenance:"
                  value={sMaintenance}
                  total={totalDevices}
                  onClick={() => handleUnhealthyClick('maintenance')}
                  dotColor={theme.palette.warning.main}
                  loading={isLoading}
                />
              </Box>

              <SmallMetricItem
                label="UFS Inventory Issues:"
                value={ufsNeedsRepair + ufsMissing}
                total={totalDevices}
                onClick={handleUfsIssuesClick}
                dotColor={theme.palette.error.main}
                loading={isLoading}
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  label="Needs Repair (UFS):"
                  value={ufsNeedsRepair}
                  total={totalDevices}
                  onClick={() => {
                    setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, []);
                    setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, [
                      'NEEDS_REPAIR',
                    ]);
                  }}
                  dotColor={theme.palette.error.main}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Missing (UFS):"
                  value={ufsMissing}
                  total={totalDevices}
                  onClick={() => {
                    setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, []);
                    setFilterOptions(`${BROWSER_UFS_SOURCE}."resource_state"`, [
                      'MISSING',
                    ]);
                  }}
                  dotColor={theme.palette.error.main}
                  loading={isLoading}
                />
              </Box>
            </SubMetricList>
          </Grid>

          {/* Column 4: Other */}
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col4}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Other"
                value={isLoading ? undefined : otherCount}
                total={totalDevices}
                loading={isLoading}
                Icon={
                  <WarningIcon
                    sx={{ color: theme.palette.warning.main, mt: '-2px' }}
                  />
                }
              />
              <InfoTooltip>
                <Typography variant="body2">
                  Devices that are excluded or inactive. Includes UFS states
                  Reserved, Decommissioned, Deployed Testing, Deploying,
                  Registered, and serving UFS devices missing Swarming bots.
                </Typography>
              </InfoTooltip>
            </Box>

            <SubMetricList>
              <SmallMetricItem
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <span>Reserved / Excluded:</span>
                    <InfoTooltip>
                      <Typography variant="body2">
                        Devices intentionally taken out of active test
                        scheduling (UFS state is Reserved, Decommissioned,
                        Deployed Testing, Deploying, or Registered).
                      </Typography>
                    </InfoTooltip>
                  </Box>
                }
                ariaLabel="Reserved or Excluded"
                value={ufsExcluded + nonServingOthers}
                total={totalDevices}
                onClick={handleNonServingClick}
                dotColor={theme.palette.grey[400]}
                loading={isLoading}
              />
              <SmallMetricItem
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <span>Missing Bot:</span>
                    <InfoTooltip>
                      <Typography variant="body2">
                        Serving devices that are registered in UFS but have no
                        connected Swarming bot client checking in.
                      </Typography>
                    </InfoTooltip>
                  </Box>
                }
                ariaLabel="Missing Swarming Bot"
                value={sMissingBots}
                total={totalDevices}
                dotColor={theme.palette.grey[400]}
                loading={isLoading}
              />
            </SubMetricList>
          </Grid>
        </Grid>
      </Box>
    );
  };

  return (
    <MetricsContainer>
      <SummaryHeaderTitle>Device Health Summary</SummaryHeaderTitle>
      {getContent()}
    </MetricsContainer>
  );
}
