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
  Card,
  CardContent,
  CardHeader,
  Divider,
  LinearProgress,
  List,
  ListItem,
  Skeleton,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import { TrendlineGrouping } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { calculate24hDelta, getHealthColor } from './ranking_utils';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

export type RankingEntityTab = 'models' | 'pools';

export interface PerformanceRankingCardProps {
  filter?: string;
  onSelectModel?: (model: string) => void;
  onSelectPool?: (pool: string) => void;
}

export interface RankedCohortItem {
  readonly name: string;
  readonly availabilityPct: number;
  readonly prevAvailabilityPct: number;
  readonly healthDrop: number;
  readonly trend: 'up' | 'down' | 'flat';
}

export const PerformanceRankingCard = ({
  filter = '',
  onSelectModel,
  onSelectPool,
}: PerformanceRankingCardProps) => {
  const [entityTab, setEntityTab] = useState<RankingEntityTab>('models');

  const queryRequest = useMemo(
    () => ({
      grouping:
        entityTab === 'models'
          ? TrendlineGrouping.GROUP_BY_MODEL
          : TrendlineGrouping.GROUP_BY_POOL,
      startTime: undefined,
      endTime: undefined,
      filter,
    }),
    [entityTab, filter],
  );

  // TODO: Transitional data source using GetFleetAvailabilityTrends;
  // switch to QueryPerformanceRankings RPC once implemented (Phase 2 / Story 8).
  const { data, isLoading, isError } = useFleetAvailabilityTrends(queryRequest);

  // TODO: Transitional data source using GetFleetAvailabilityTrends;
  // switch to QueryPerformanceRankings RPC once implemented (Phase 2 / Story 8).
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

    return list.sort((a, b) => a.availabilityPct - b.availabilityPct);
  }, [data?.series]);

  const onSelectItem = entityTab === 'models' ? onSelectModel : onSelectPool;

  return (
    <Card
      variant="outlined"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        flexGrow: 1,
        height: '100%',
        minHeight: { xs: 560, lg: 0 },
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
            }}
          >
            <Tabs
              value={entityTab}
              onChange={(_, val: RankingEntityTab | null) => {
                if (val !== null) setEntityTab(val);
              }}
              aria-label="Performance ranking tabs"
            >
              <Tab
                value="models"
                label="Models"
                sx={{ textTransform: 'none', fontWeight: 600, minWidth: 80 }}
              />
              <Tab
                value="pools"
                label="Pools"
                sx={{ textTransform: 'none', fontWeight: 600, minWidth: 80 }}
              />
            </Tabs>
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
                onClick={() => onSelectItem?.(item.name)}
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
                    <Typography
                      variant="caption"
                      sx={{
                        fontWeight: 'bold',
                        color: 'text.primary',
                        fontSize: '0.72rem',
                      }}
                    >
                      {item.availabilityPct}%{' '}
                      {entityTab === 'models' ? 'Available' : 'Healthy'}
                    </Typography>
                    {item.trend === 'down' && (
                      <Box
                        sx={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 0.25,
                        }}
                      >
                        <TrendingDown color="error" sx={{ fontSize: 16 }} />
                        <Typography
                          variant="caption"
                          data-testid={`drop-badge-${item.name}`}
                          sx={{
                            color: 'error.main',
                            fontWeight: 'bold',
                            fontSize: '0.75rem',
                          }}
                        >
                          -{item.healthDrop}%
                        </Typography>
                      </Box>
                    )}
                    {item.trend === 'up' && (
                      <TrendingUp color="success" sx={{ fontSize: 16 }} />
                    )}
                    {item.trend === 'flat' && (
                      <TrendingFlat color="action" sx={{ fontSize: 16 }} />
                    )}
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={item.availabilityPct}
                    color={getHealthColor(item.availabilityPct)}
                    sx={{ height: 6, borderRadius: 3 }}
                    aria-label={`${item.name} availability`}
                  />
                </Box>
              </ListItem>
            ))}
          </List>
        )}
      </CardContent>
    </Card>
  );
};
