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

import { Box, Chip, Tooltip, Typography } from '@mui/material';

import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { borders, fleetColors, fontSizes, radii } from '@/fleet/theme/tokens';

import { formatPoints, ParsedPriorityRule } from './utils';

/**
 * The fields of a repair queue item that the priority score cell reads.
 * Structurally compatible with the per-platform repair queue item protos.
 */
export interface PriorityScoredItem {
  readonly priorityScore: string;
  readonly matchedRuleIds: readonly string[];
}

export interface PriorityScoreCellProps {
  readonly item: PriorityScoredItem;
  readonly rulesById: ReadonlyMap<string, ParsedPriorityRule>;
  readonly open?: boolean;
}

export const PriorityScoreCell = ({
  item,
  rulesById,
  open,
}: PriorityScoreCellProps) => {
  const scoreStr = item.priorityScore || '0';
  const formattedScore = formatPoints(scoreStr);
  const matchedIds = item.matchedRuleIds || [];

  let tooltipContent: React.ReactNode;
  if (matchedIds.length === 0) {
    tooltipContent = (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 0.5,
          minWidth: 220,
        }}
      >
        <Typography
          variant="subtitle2"
          sx={{ fontWeight: 600, color: fleetColors.textPrimary }}
        >
          No matched priority rules (0 pts)
        </Typography>
        <Typography
          variant="caption"
          sx={{ color: fleetColors.textSecondary, lineHeight: 1.4 }}
        >
          This device is operating at baseline priority with no matching active
          rules.
        </Typography>
      </Box>
    );
  } else {
    const cleanScore = scoreStr.replace(/^\+/, '');
    tooltipContent = (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 1.25,
          minWidth: 280,
          maxWidth: 420,
        }}
      >
        {/* Header */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 2,
            pb: 0.75,
            borderBottom: borders.divider,
          }}
        >
          <Typography
            variant="subtitle2"
            sx={{ fontWeight: 600, color: fleetColors.textPrimary }}
          >
            Priority score breakdown
          </Typography>
          <Typography
            variant="caption"
            sx={{
              color: fleetColors.textSecondary,
              fontWeight: 500,
              bgcolor: fleetColors.surfaceTonal,
              px: 1,
              py: 0.25,
              borderRadius: radii.pill,
            }}
          >
            {matchedIds.length}{' '}
            {matchedIds.length === 1 ? 'rule matched' : 'rules matched'}
          </Typography>
        </Box>

        {/* Rule list */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          {matchedIds.map((id) => {
            const rule = rulesById.get(id);
            const tooltipTitle = rule?.expressionAip160 ?? `Rule #${id}`;
            const weightNum = rule ? Number(rule.weight) : 0;
            const isPositive = weightNum > 0;
            const isNegative = weightNum < 0;
            const weightStr = rule ? formatPoints(rule.weight) : '';

            return (
              <Box
                key={id}
                data-testid={`matched-rule-${id}`}
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 1.5,
                }}
              >
                {/* Condition chips */}
                <Box
                  sx={{
                    display: 'flex',
                    flexWrap: 'wrap',
                    gap: 0.5,
                    alignItems: 'center',
                    flex: 1,
                  }}
                  title={tooltipTitle}
                >
                  {rule?.chips ?? (
                    <Chip
                      variant="filter"
                      label={tooltipTitle}
                      sx={{ cursor: 'default' }}
                    />
                  )}
                </Box>

                {/* Points badge */}
                {weightStr && (
                  <StatusPill
                    tone={
                      isPositive ? 'success' : isNegative ? 'error' : 'neutral'
                    }
                    dot={false}
                    size="compact"
                  >
                    {weightStr}
                  </StatusPill>
                )}
              </Box>
            );
          })}
        </Box>

        {/* Total Summary Footer */}
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 2,
            pt: 0.75,
            borderTop: borders.divider,
          }}
        >
          <Typography
            variant="caption"
            sx={{ fontWeight: 600, color: fleetColors.textSecondary }}
          >
            Total score
          </Typography>
          <Box
            component="span"
            sx={{
              display: 'inline-flex',
              alignItems: 'center',
              px: 1,
              py: 0.25,
              borderRadius: radii.pill,
              bgcolor: fleetColors.surfaceTonal,
              fontWeight: 700,
              fontSize: fontSizes.caption,
              color: fleetColors.textPrimary,
              fontVariantNumeric: 'tabular-nums',
              whiteSpace: 'nowrap',
            }}
          >
            {`= ${cleanScore} pts`}
          </Box>
        </Box>
      </Box>
    );
  }

  return (
    <Tooltip
      title={tooltipContent}
      open={open}
      enterDelay={100}
      placement="bottom-start"
      slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
    >
      <Typography
        component="span"
        variant="body2"
        tabIndex={0}
        sx={{
          fontSize: fontSizes.label,
          fontWeight: 600,
          cursor: 'help',
          display: 'inline-block',
          textDecoration: 'underline dotted',
          textUnderlineOffset: '3px',
        }}
      >
        {formattedScore}
      </Typography>
    </Tooltip>
  );
};
