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
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { getMetricsGridStyles } from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useBrowserFilters } from './use_browser_filters';

export function BrowserSummaryHeader() {
  const client = useFleetConsoleClient();
  const theme = useTheme();
  const { filterValues, aip160 } = useBrowserFilters();

  const gridStyles = getMetricsGridStyles(theme);

  // Global query matching current active page filters
  const countQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: aip160(),
    }),
  );

  const userFilter = aip160();

  // Subset query restricted to serving devices
  const servingFilter = userFilter
    ? `(${userFilter}) AND (ufs.resource_state = "SERVING")`
    : 'ufs.resource_state = "SERVING"';

  const servingQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: servingFilter,
    }),
  );

  // Subset query restricted to NEEDS_REPAIR devices
  const repairFilter = userFilter
    ? `(${userFilter}) AND (ufs.resource_state = "NEEDS_REPAIR")`
    : 'ufs.resource_state = "NEEDS_REPAIR"';

  const repairQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: repairFilter,
    }),
  );

  // Subset query restricted to MISSING devices
  const missingFilter = userFilter
    ? `(${userFilter}) AND (ufs.resource_state = "MISSING")`
    : 'ufs.resource_state = "MISSING"';

  const missingQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: missingFilter,
    }),
  );

  // Subset query restricted to excluded UFS states (Reserved, Decommissioned, Deployed Testing, Deploying, Registered)
  const excludedFilter = userFilter
    ? `(${userFilter}) AND (ufs.resource_state = "RESERVED" OR ` +
      `ufs.resource_state = "DECOMMISSIONED" OR ` +
      `ufs.resource_state = "DEPLOYED_TESTING" OR ` +
      `ufs.resource_state = "DEPLOYING" OR ` +
      `ufs.resource_state = "REGISTERED")`
    : `ufs.resource_state = "RESERVED" OR ` +
      `ufs.resource_state = "DECOMMISSIONED" OR ` +
      `ufs.resource_state = "DEPLOYED_TESTING" OR ` +
      `ufs.resource_state = "DEPLOYING" OR ` +
      `ufs.resource_state = "REGISTERED"`;

  const excludedQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: excludedFilter,
    }),
  );

  const setFilterOptions = (key: string, options: string[]) => {
    const filter = filterValues?.[key];
    if (filter instanceof StringListFilterCategory) {
      filter.setSelectedOptions(options);
    }
  };

  const getContent = () => {
    if (
      countQuery.isError ||
      servingQuery.isError ||
      repairQuery.isError ||
      missingQuery.isError ||
      excludedQuery.isError
    ) {
      return (
        <Alert severity="error">
          {getErrorMessage(
            countQuery.error ||
              servingQuery.error ||
              repairQuery.error ||
              missingQuery.error ||
              excludedQuery.error,
            'get the main metrics',
          )}
        </Alert>
      );
    }

    const isLoading =
      countQuery.isPending ||
      servingQuery.isPending ||
      repairQuery.isPending ||
      missingQuery.isPending ||
      excludedQuery.isPending ||
      !countQuery.data ||
      !servingQuery.data ||
      !repairQuery.data ||
      !missingQuery.data ||
      !excludedQuery.data;

    const totalDevices = countQuery.data?.total ?? 0;
    const swTotal = countQuery.data?.swarmingState?.total ?? 0;

    // Subset counts:
    const sTotal = servingQuery.data?.total ?? 0;
    const sAlive = servingQuery.data?.swarmingState?.alive ?? 0;
    const sDead = servingQuery.data?.swarmingState?.dead ?? 0;
    const sQuarantined = servingQuery.data?.swarmingState?.quarantined ?? 0;
    const sMaintenance = servingQuery.data?.swarmingState?.maintenance ?? 0;

    const ufsNeedsRepair = repairQuery.data?.total ?? 0;
    const ufsMissing = missingQuery.data?.total ?? 0;
    const ufsExcluded = excludedQuery.data?.total ?? 0;

    // ACCOUNTING RATIONALE & SHORTCOMINGS:
    // 1. UFS vs Swarming Nature:
    //    - UFS resource states (SERVING, NEEDS_REPAIR, MISSING, EXCLUDED) are mutually exclusive.
    //      A device has exactly one UFS state.
    //    - Swarming states (alive, dead, quarantined, maintenance) are attribute flags.
    //      A single bot can carry multiple flags simultaneously (e.g., dead + quarantined,
    //      alive + maintenance, or alive + quarantined).
    // 2. Shortcoming of Current CountBrowserDevices API:
    //    - CountBrowserDevices.swarmingState returns independent marginal counts per flag.
    //      Because multi-flag bots appear in multiple marginal totals, the sum
    //      (sDead + sQuarantined + sMaintenance) can exceed total broken bots or overlap with sAlive.
    // 3. Top-Level Mutually Exclusive Clamping (100% Fleet Accounting):
    //    - Healthy: A SERVING UFS device is only truly healthy if its bot is Alive and free of
    //      broken flags (quarantined or maintenance). We subtract sQuarantined and sMaintenance
    //      from sAlive and clamp with Math.max(0, ...) so overlapping flags never cause negative counts.
    //    - Other: Cleanly partitions non-serving UFS states (EXCLUDED + other non-serving) plus
    //      SERVING devices missing bot data (sMissingBots).
    //    - Unhealthy: Aggregates broken Swarming marginal counts + broken UFS states (NEEDS_REPAIR, MISSING).
    // 4. Drill-down Items (Marginal Counts):
    //    - Within the expanded scorecard list, displaying exact marginal counts (sDead, sQuarantined,
    //      sMaintenance) ensures that clicking a filter item shows 100% of bots with that failure flag.
    // TODO(b/524930584): Long-term, extend CountBrowserDevices backend or multi-dimensional group_by
    // to return mutually exclusive composite bot health categories (e.g., applying strict precedence
    // DEAD > QUARANTINED > MAINTENANCE > HEALTHY on backend) so the frontend receives exact
    // non-overlapping partition counts without client-side clamping.

    // Healthy: SERVING state in UFS AND Alive in Swarming (excluding quarantined/maintenance)
    const healthyCount = Math.max(0, sAlive - sQuarantined - sMaintenance);
    // Unhealthy (Broken): expected serving but failed + UFS states indicating issues (NEEDS_REPAIR, MISSING)
    const unhealthyCount =
      sDead + sQuarantined + sMaintenance + ufsNeedsRepair + ufsMissing;
    // Other (Reserved / Excluded): Excluded UFS states + serving devices missing bots + unspecified remaining
    const sMissingBots = Math.max(
      0,
      sTotal - (servingQuery.data?.swarmingState?.total ?? 0),
    );
    const nonServingOthers = Math.max(
      0,
      totalDevices - sTotal - ufsNeedsRepair - ufsMissing - ufsExcluded,
    );
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

            <Grid container spacing={1} sx={{ mt: 1, px: 1 }}>
              <Grid item xs={12}>
                <SmallMetricItem
                  label="With Bots:"
                  value={swTotal}
                  total={totalDevices}
                  dotColor={theme.palette.success.main}
                  loading={isLoading}
                />
              </Grid>
            </Grid>
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

            <Grid container spacing={1} sx={{ mt: 1, px: 1 }}>
              <Grid item xs={12}>
                <SmallMetricItem
                  label="Alive & Serving:"
                  value={healthyCount}
                  total={totalDevices}
                  onClick={handleHealthyClick}
                  dotColor={theme.palette.success.main}
                  loading={isLoading}
                />
              </Grid>
            </Grid>
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

            <Grid container spacing={1} sx={{ mt: 1, px: 1 }}>
              <Grid item xs={12}>
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
                    dotColor={colors.yellow[900]}
                    loading={isLoading}
                  />
                  <SmallMetricItem
                    label="In Maintenance:"
                    value={sMaintenance}
                    total={totalDevices}
                    onClick={() => handleUnhealthyClick('maintenance')}
                    dotColor={colors.yellow[900]}
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
                      setFilterOptions(
                        `${BROWSER_SWARMING_SOURCE}."state"`,
                        [],
                      );
                      setFilterOptions(
                        `${BROWSER_UFS_SOURCE}."resource_state"`,
                        ['NEEDS_REPAIR'],
                      );
                    }}
                    dotColor={theme.palette.error.main}
                    loading={isLoading}
                  />
                  <SmallMetricItem
                    label="Missing (UFS):"
                    value={ufsMissing}
                    total={totalDevices}
                    onClick={() => {
                      setFilterOptions(
                        `${BROWSER_SWARMING_SOURCE}."state"`,
                        [],
                      );
                      setFilterOptions(
                        `${BROWSER_UFS_SOURCE}."resource_state"`,
                        ['MISSING'],
                      );
                    }}
                    dotColor={theme.palette.error.main}
                    loading={isLoading}
                  />
                </Box>
              </Grid>
            </Grid>
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
                  <WarningIcon sx={{ color: colors.yellow[900], mt: '-2px' }} />
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

            <Grid container spacing={1} sx={{ mt: 1, px: 1 }}>
              <Grid item xs={12}>
                <SmallMetricItem
                  label={
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
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
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
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
              </Grid>
            </Grid>
          </Grid>
        </Grid>
      </Box>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h5" sx={{ mb: 2 }}>
        Device Health Summary
      </Typography>
      {getContent()}
    </MetricsContainer>
  );
}
