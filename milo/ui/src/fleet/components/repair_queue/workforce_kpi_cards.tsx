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

import FormatListBulletedIcon from '@mui/icons-material/FormatListBulleted';
import GroupIcon from '@mui/icons-material/Group';
import HowToRegIcon from '@mui/icons-material/HowToReg';
import TimerIcon from '@mui/icons-material/Timer';
import TrendingUpIcon from '@mui/icons-material/TrendingUp';
import { Box, Paper, Skeleton, Typography } from '@mui/material';
import { ElementType, ReactNode } from 'react';

import { colors } from '@/fleet/theme/colors';

import {
  DEFAULT_WORKFORCE_LABELS,
  formatWorkforceDuration,
  TimeframeOption,
  WorkforceActivityData,
} from './workforce_helpers';

export interface WorkforceKpiCardsProps {
  data?: WorkforceActivityData;
  loading: boolean;
  timeframe: TimeframeOption;
  showPickupRank: boolean;
  showInProgressRepairs: boolean;
  inProgressSubtitle?: string;
}

export const WorkforceKpiCards = ({
  data,
  loading,
  timeframe,
  showPickupRank,
  showInProgressRepairs,
  inProgressSubtitle = DEFAULT_WORKFORCE_LABELS.inProgressSubtitle,
}: WorkforceKpiCardsProps) => (
  <Box
    sx={{
      display: 'grid',
      gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))',
      gap: 2,
      mb: 3,
    }}
  >
    <KpiCard
      title="Active Repairers"
      Icon={GroupIcon}
      value={data?.activeRepairers ?? 0}
      subtitle={`Contributing in selected window (${timeframe})`}
      loading={loading}
      skeletonWidth={60}
    />
    <KpiCard
      title="Total Priority Points Cleared"
      Icon={TrendingUpIcon}
      value={`${(data?.totalPriorityPointsCleared ?? 0).toLocaleString()} pts`}
      subtitle={`High-impact score restored (${timeframe})`}
      loading={loading}
      skeletonWidth={100}
      valueColor="primary.main"
    />
    {showPickupRank && (
      <KpiCard
        title="Avg Queue Pickup Rank"
        Icon={FormatListBulletedIcon}
        value={`#${data?.avgQueuePickupRank ?? 0}`}
        subtitle="Mean pickup position (Anti-skew)"
        loading={loading}
        skeletonWidth={70}
        valueColor="primary.main"
      />
    )}
    {showInProgressRepairs && (
      <KpiCard
        title="In-Progress Repairs"
        Icon={HowToRegIcon}
        value={data?.inProgressRepairs ?? 0}
        subtitle={inProgressSubtitle}
        loading={loading}
        skeletonWidth={60}
        valueColor={colors.orange[600]}
      />
    )}
    <KpiCard
      title="Avg Repair Time (MTTR)"
      Icon={TimerIcon}
      value={formatWorkforceDuration(data?.avgRepairTime)}
      subtitle={`Mean elapsed time (${timeframe})`}
      loading={loading}
      skeletonWidth={90}
      valueColor="primary.main"
    />
  </Box>
);

interface KpiCardProps {
  title: string;
  Icon: ElementType;
  value: ReactNode;
  subtitle: string;
  loading: boolean;
  skeletonWidth: number;
  valueColor?: string;
}

const KpiCard = ({
  title,
  Icon,
  value,
  subtitle,
  loading,
  skeletonWidth,
  valueColor,
}: KpiCardProps) => (
  <Paper
    elevation={0}
    sx={{
      p: 2,
      borderRadius: '8px',
      border: `1px solid ${colors.grey[300]}`,
    }}
  >
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        mb: 1,
      }}
    >
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ fontWeight: 600 }}
      >
        {title}
      </Typography>
      <Icon sx={{ color: valueColor ?? 'primary.main', fontSize: 20 }} />
    </Box>
    {loading ? (
      <Skeleton
        variant="text"
        width={skeletonWidth}
        height={42}
        sx={{ mb: 0.5 }}
        data-testid="kpi-skeleton"
      />
    ) : (
      <Typography
        variant="h4"
        sx={{
          fontWeight: 700,
          mb: 0.5,
          ...(valueColor ? { color: valueColor } : {}),
        }}
      >
        {value}
      </Typography>
    )}
    <Typography variant="caption" color="text.secondary">
      {subtitle}
    </Typography>
  </Paper>
);
