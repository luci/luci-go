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

import { TrendingDown, TrendingFlat, TrendingUp } from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Divider,
  FormControl,
  LinearProgress,
  List,
  ListItem,
  MenuItem,
  Select,
  Skeleton,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import {
  CHROMEOS_PLATFORM,
  generateDeviceListURL,
} from '@/fleet/constants/paths';
import { getFilterKey } from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';
import { subMetricRowSx } from '@/fleet/theme/recipes';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { TrendlineGrouping } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { HEALTH_FILTER_CONFIGS } from './filter_constants';
import { getHealthStatus } from './health_status_utils';
import { HealthThresholdsTooltip, HealthTooltip } from './health_tooltip';
import {
  calculate24hDelta,
  DeltaCalculationResult,
  HEALTH_COMPARISON_TOOLTIP,
} from './ranking_utils';
import {
  getHourAlignedTimeWindow,
  RANKING_HOURS_WINDOW,
} from './time_window_utils';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

export type RankingEntityTab = 'models' | 'pools';
export type RankingSortOption = 'availability' | 'drop';

export interface PerformanceRankingCardProps {
  filter?: string;
  onSelectModel?: (model: string) => void;
  onSelectPool?: (pool: string) => void;
}

export interface RankedCohortItem extends DeltaCalculationResult {
  readonly name: string;
}

export const PerformanceRankingCard = ({
  filter = '',
  onSelectModel,
  onSelectPool,
}: PerformanceRankingCardProps) => {
  const [entityTab, setEntityTab] = useState<RankingEntityTab>('models');
  const [sortBy, setSortBy] = useState<RankingSortOption>('availability');

  const queryRequest = useMemo(() => {
    const { startTime, endTime } =
      getHourAlignedTimeWindow(RANKING_HOURS_WINDOW);
    return {
      grouping:
        entityTab === 'models'
          ? TrendlineGrouping.GROUP_BY_MODEL
          : TrendlineGrouping.GROUP_BY_POOL,
      startTime,
      endTime,
      filter,
    };
  }, [entityTab, filter]);

  // Uses GetFleetAvailabilityTrends until the dedicated QueryPerformanceRankings RPC lands.
  const { data, isLoading, isError } = useFleetAvailabilityTrends(queryRequest);

  const rankedItems = useMemo<readonly RankedCohortItem[]>(() => {
    if (!data?.series) return [];

    const list: RankedCohortItem[] = [];
    for (const s of data.series) {
      if (!s.name || s.name === 'Overall') continue;
      const delta = calculate24hDelta(s.points ?? []);
      list.push({
        name: s.name,
        ...delta,
      });
    }

    if (sortBy === 'drop') {
      return list.sort((a, b) => b.healthDrop - a.healthDrop);
    }

    return list.sort((a, b) => a.availabilityPct - b.availabilityPct);
  }, [data?.series, sortBy]);

  const onSelectItem = entityTab === 'models' ? onSelectModel : onSelectPool;

  // Trend series names are lowercased by the backend (e.g. "anahera"), while
  // device dimension values keep their original casing (e.g. "Anahera").
  // Filters match option values exactly, so map to the canonical value.
  //
  // Known limitation: device labels are not consistently cased in production
  // (e.g. both "VOLET" and "volet" exist). Health aggregates them into one
  // lowercase series, but this maps it to only one of the casings, so the
  // device list shows a subset of the devices counted here. There is
  // intentionally no fallback for multiple casings until a fleet-wide strategy
  // is decided.
  //
  // TODO(b/568309497): Revisit once model label casing is handled
  // consistently across fleetconsole.
  const { getValues } = useChromeOSFields();
  const dimensionKey =
    entityTab === 'models'
      ? HEALTH_FILTER_CONFIGS.MODEL.dimensionSource
      : HEALTH_FILTER_CONFIGS.POOL.dimensionSource;
  const canonicalNames = useMemo(
    () => new Map(getValues(dimensionKey).map((v) => [v.toLowerCase(), v])),
    [getValues, dimensionKey],
  );
  const toCanonical = (name: string) =>
    canonicalNames.get(name.toLowerCase()) ?? name;

  return (
    <Card
      variant="outlined"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        flexGrow: 1,
        height: '100%',
        minHeight: { xs: 560, lg: 0 },
        maxHeight: { xs: 560, lg: 'none' },
        boxSizing: 'border-box',
        borderRadius: 2,
        bgcolor: '#ffffff',
        boxShadow: '0 1px 3px rgba(0,0,0,0.05)',
      }}
    >
      <CardHeader
        title={
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              width: '100%',
              gap: 1,
              flexWrap: 'wrap',
            }}
          >
            <Tabs
              value={entityTab}
              onChange={(_, val: RankingEntityTab | null) => {
                if (val !== null) setEntityTab(val);
              }}
              aria-label="Performance ranking tabs"
              sx={{ minHeight: 36 }}
            >
              <Tab
                value="models"
                label="Models"
                sx={{
                  textTransform: 'none',
                  fontWeight: 600,
                  minWidth: 0,
                  minHeight: 36,
                  px: 1.25,
                  py: 0.5,
                }}
              />
              <Tab
                value="pools"
                label="Pools"
                sx={{
                  textTransform: 'none',
                  fontWeight: 600,
                  minWidth: 0,
                  minHeight: 36,
                  px: 1.25,
                  py: 0.5,
                }}
              />
            </Tabs>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{
                  fontWeight: 600,
                  fontSize: '0.8125rem',
                  whiteSpace: 'nowrap',
                }}
              >
                Sort by:
              </Typography>
              <FormControl size="small" sx={{ minWidth: 110 }}>
                <Select
                  inputProps={{ 'aria-label': 'Sort by' }}
                  size="small"
                  value={sortBy}
                  onChange={(e) =>
                    setSortBy(e.target.value as RankingSortOption)
                  }
                  sx={{
                    height: 28,
                    fontSize: '0.8125rem',
                    bgcolor: '#ffffff',
                    '& .MuiSelect-select': {
                      py: '2px !important',
                      pl: 1.25,
                      pr: '28px !important',
                      fontSize: '0.8125rem',
                      lineHeight: '24px',
                      display: 'flex',
                      alignItems: 'center',
                    },
                    '& .MuiOutlinedInput-notchedOutline': {
                      borderColor: 'divider',
                    },
                    '& .MuiSvgIcon-root': {
                      fontSize: '1.1rem',
                    },
                  }}
                >
                  <MenuItem
                    value="availability"
                    sx={{ fontSize: '0.8125rem', py: 0.5, minHeight: 30 }}
                  >
                    Availability
                  </MenuItem>
                  <MenuItem
                    value="drop"
                    sx={{ fontSize: '0.8125rem', py: 0.5, minHeight: 30 }}
                  >
                    Health Drop
                  </MenuItem>
                </Select>
              </FormControl>
            </Box>
          </Box>
        }
        sx={{ py: 1, px: 2 }}
      />
      <Divider />
      <CardContent
        sx={{
          p: 0,
          overflowY: 'auto',
          flexGrow: 1,
          minHeight: 0,
          scrollbarWidth: 'thin',
          '&::-webkit-scrollbar': { width: '6px' },
          '&::-webkit-scrollbar-thumb': {
            backgroundColor: 'divider',
            borderRadius: '4px',
          },
          '&:last-child': { pb: 0 },
        }}
      >
        {isLoading && (
          <Box sx={{ p: 2 }}>
            <Skeleton variant="text" height={36} />
            <Skeleton
              variant="rectangular"
              height={8}
              sx={{ mb: 2, borderRadius: 1 }}
            />
            <Skeleton variant="text" height={36} />
            <Skeleton
              variant="rectangular"
              height={8}
              sx={{ mb: 2, borderRadius: 1 }}
            />
            <Skeleton variant="text" height={36} />
            <Skeleton
              variant="rectangular"
              height={8}
              sx={{ borderRadius: 1 }}
            />
          </Box>
        )}

        {isError && (
          <Alert severity="error" sx={{ m: 2 }}>
            Failed to load hardware performance ranking.
          </Alert>
        )}

        {!isLoading && !isError && rankedItems.length === 0 && (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              {entityTab === 'models'
                ? 'No models found matching current filters.'
                : 'No pools found matching current filters.'}
            </Typography>
          </Box>
        )}

        {!isLoading && !isError && rankedItems.length > 0 && (
          <List dense disablePadding>
            {rankedItems.map((item) => (
              <ListItem
                key={item.name}
                data-testid={`ranking-row-${item.name}`}
                onClick={() => onSelectItem?.(toCanonical(item.name))}
                sx={{
                  py: 1.5,
                  px: 2.5,
                  borderBottom: '1px solid',
                  borderColor: 'divider',
                  '&:last-child': { borderBottom: 'none' },
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'stretch',
                  gap: 1,
                  cursor: onSelectItem ? 'pointer' : 'default',
                  '&:hover': onSelectItem
                    ? { bgcolor: 'action.hover' }
                    : undefined,
                }}
              >
                <Box
                  sx={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
                  <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
                    {item.name}
                  </Typography>
                  <Button
                    size="small"
                    variant="outlined"
                    color="primary"
                    component="a"
                    href={
                      generateDeviceListURL(CHROMEOS_PLATFORM) +
                      getFilterQueryString(
                        {
                          [getFilterKey(
                            entityTab === 'models'
                              ? 'label-model'
                              : 'label-pool',
                          )]: [toCanonical(item.name)],
                        },
                        undefined,
                      )
                    }
                    target="_blank"
                    rel="noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    sx={{
                      textTransform: 'none',
                      fontSize: 10,
                      py: 0.1,
                      px: 0.8,
                      height: 20,
                      borderRadius: 1,
                    }}
                  >
                    View devices
                  </Button>
                </Box>
                <Box sx={{ width: '100%', mt: 0.25 }}>
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.75,
                      mb: 0.5,
                    }}
                  >
                    <HealthThresholdsTooltip
                      pct={item.availabilityPct}
                      metricLabel={
                        entityTab === 'models' ? 'Availability' : 'Health'
                      }
                    >
                      <Typography
                        variant="caption"
                        data-testid={`availability-label-${item.name}`}
                        tabIndex={0}
                        sx={{
                          ...subMetricRowSx,
                          fontWeight: 'bold',
                          color: 'text.primary',
                          fontSize: '0.72rem',
                          cursor: 'pointer',
                          px: 0.5,
                          py: 0.25,
                        }}
                      >
                        {item.availabilityPct}%{' '}
                        {entityTab === 'models' ? 'Available' : 'Healthy'}
                      </Typography>
                    </HealthThresholdsTooltip>
                    {item.trend === 'down' && (
                      <HealthTooltip title={HEALTH_COMPARISON_TOOLTIP}>
                        <Box
                          tabIndex={0}
                          data-testid={`drop-badge-${item.name}`}
                          sx={{
                            ...subMetricRowSx,
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 0.25,
                            cursor: 'pointer',
                            px: 0.5,
                            py: 0.25,
                          }}
                        >
                          <TrendingDown color="error" sx={{ fontSize: 16 }} />
                          <Typography
                            variant="caption"
                            sx={{
                              color: 'error.main',
                              fontWeight: 'bold',
                              fontSize: '0.75rem',
                            }}
                          >
                            -{item.healthDrop}%
                          </Typography>
                        </Box>
                      </HealthTooltip>
                    )}
                    {item.trend === 'up' && (
                      <HealthTooltip title={HEALTH_COMPARISON_TOOLTIP}>
                        <Box
                          component="span"
                          tabIndex={0}
                          data-testid={`gain-badge-${item.name}`}
                          sx={{
                            ...subMetricRowSx,
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 0.25,
                            cursor: 'pointer',
                            px: 0.5,
                            py: 0.25,
                          }}
                        >
                          <TrendingUp color="success" sx={{ fontSize: 16 }} />
                          <Typography
                            variant="caption"
                            sx={{
                              color: 'success.main',
                              fontWeight: 'bold',
                              fontSize: '0.75rem',
                            }}
                          >
                            +{item.healthGain}%
                          </Typography>
                        </Box>
                      </HealthTooltip>
                    )}
                    {item.trend === 'flat' && (
                      <HealthTooltip title={HEALTH_COMPARISON_TOOLTIP}>
                        <Box
                          component="span"
                          tabIndex={0}
                          data-testid={`flat-badge-${item.name}`}
                          sx={{
                            ...subMetricRowSx,
                            display: 'inline-flex',
                            alignItems: 'center',
                            cursor: 'pointer',
                            px: 0.5,
                            py: 0.25,
                          }}
                        >
                          <TrendingFlat color="action" sx={{ fontSize: 16 }} />
                        </Box>
                      </HealthTooltip>
                    )}
                  </Box>
                  <HealthThresholdsTooltip
                    pct={item.availabilityPct}
                    metricLabel={
                      entityTab === 'models' ? 'Availability' : 'Health'
                    }
                  >
                    <Box
                      tabIndex={0}
                      sx={{
                        ...subMetricRowSx,
                        cursor: 'pointer',
                        py: 0.5,
                        '&:hover .MuiLinearProgress-root, &:focus-visible .MuiLinearProgress-root':
                          {
                            transform: 'scaleY(1.33)',
                            filter: 'brightness(1.08)',
                            boxShadow: '0 2px 6px rgba(0,0,0,0.12)',
                          },
                      }}
                    >
                      <LinearProgress
                        variant="determinate"
                        value={Math.min(100, item.availabilityPct)}
                        color={getHealthStatus(item.availabilityPct).color}
                        sx={{
                          height: 6,
                          borderRadius: 3,
                          transition:
                            'transform 0.15s cubic-bezier(0.4, 0, 0.2, 1), filter 0.15s ease, box-shadow 0.15s ease',
                        }}
                        aria-label={`${item.name} availability`}
                        data-testid={`progress-bar-${item.name}`}
                      />
                    </Box>
                  </HealthThresholdsTooltip>
                </Box>
              </ListItem>
            ))}
          </List>
        )}
      </CardContent>
    </Card>
  );
};
