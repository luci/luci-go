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

import { Box, Divider, Tooltip, Typography } from '@mui/material';

import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
import { fontSizes } from '@/fleet/theme/tokens';

const roundPct = (pct?: number): number =>
  pct !== undefined && pct !== null ? Math.round(pct * 100) : 0;

const formatPct = (pct?: number): string =>
  pct !== undefined && pct !== null ? `${Math.round(pct * 100)}%` : '-';

export interface HealthMetricProps {
  readonly label: string;
  readonly actualPct?: number;
  readonly inRepairPct?: number;
}

export const HealthMetric = ({
  label,
  actualPct,
  inRepairPct,
}: HealthMetricProps) => {
  if (actualPct === undefined || actualPct === null) {
    return <span>-</span>;
  }
  const actualRound = Math.round(actualPct * 100);
  const actualStr = `${actualRound}%`;
  const inRepairRound = roundPct(inRepairPct);

  if (inRepairRound <= 0) {
    return <span>{actualStr}</span>;
  }

  const effectiveRound = Math.min(100, actualRound + inRepairRound);

  const tooltipContent = (
    <Box>
      <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 0.5 }}>
        {label} Health Breakdown
      </Typography>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'auto auto',
          columnGap: 2,
          rowGap: 0.25,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          Actual available:
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 500, textAlign: 'right' }}
        >
          {actualStr}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          In repair (claimed):
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 500, textAlign: 'right' }}
        >
          +{inRepairRound}%
        </Typography>
        <Divider sx={{ gridColumn: '1 / -1', my: 0.5 }} />
        <Typography variant="body2" color="text.secondary">
          Effective health:
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 600, textAlign: 'right' }}
        >
          {effectiveRound}%
        </Typography>
      </Box>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ mt: 0.75, display: 'block', fontStyle: 'italic' }}
      >
        * Effective health counts claimed devices as healthy for priority rules.
      </Typography>
    </Box>
  );

  return (
    <span>
      {actualStr}{' '}
      <Tooltip
        title={tooltipContent}
        arrow
        enterDelay={100}
        placement="bottom-start"
        slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
      >
        <Box
          component="span"
          tabIndex={0}
          data-testid={`${label.toLowerCase().replace(/\s+/g, '-')}-in-repair-badge`}
          sx={{
            borderBottom: '1px dashed currentColor',
            cursor: 'help',
            fontWeight: 500,
          }}
        >
          (+{inRepairRound}%)
        </Box>
      </Tooltip>
    </span>
  );
};

export interface HealthPairCellProps {
  readonly primaryLabel: string;
  readonly primaryHealthPct?: number;
  readonly primaryInRepairPct?: number;
  readonly secondaryLabel: string;
  readonly secondaryHealthPct?: number;
  readonly secondaryInRepairPct?: number;
}

export const HealthPairCell = ({
  primaryLabel,
  primaryHealthPct,
  primaryInRepairPct,
  secondaryLabel,
  secondaryHealthPct,
  secondaryInRepairPct,
}: HealthPairCellProps) => {
  if (
    roundPct(primaryInRepairPct) <= 0 &&
    roundPct(secondaryInRepairPct) <= 0
  ) {
    return (
      <Typography
        variant="body2"
        noWrap
        sx={{ fontSize: fontSizes.label }}
      >{`${formatPct(primaryHealthPct)} / ${formatPct(secondaryHealthPct)}`}</Typography>
    );
  }

  return (
    <Typography
      variant="body2"
      sx={{
        fontSize: fontSizes.label,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 0.5,
        whiteSpace: 'nowrap',
      }}
    >
      <HealthMetric
        label={primaryLabel}
        actualPct={primaryHealthPct}
        inRepairPct={primaryInRepairPct}
      />
      <span>/</span>
      <HealthMetric
        label={secondaryLabel}
        actualPct={secondaryHealthPct}
        inRepairPct={secondaryInRepairPct}
      />
    </Typography>
  );
};
