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
import { Fragment } from 'react';

import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';

import { RepairPriorityRule } from './editable_rule_row';
import { formatPoints } from './utils';

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
  readonly rulesById: ReadonlyMap<string, RepairPriorityRule>;
}

/**
 * Caps how wide a rule's AIP-160 expression may grow inside the breakdown
 * tooltip. Longer expressions are truncated with an ellipsis so that a single
 * verbose rule cannot stretch the tooltip across the viewport, and so the
 * points column stays anchored on the right.
 */
const RULE_EXPRESSION_MAX_WIDTH = 300;

export const PriorityScoreCell = ({
  item,
  rulesById,
}: PriorityScoreCellProps) => {
  const scoreStr = item.priorityScore || '0';
  const formattedScore = formatPoints(scoreStr);
  const matchedIds = item.matchedRuleIds || [];

  let tooltipContent: React.ReactNode;
  if (matchedIds.length === 0) {
    tooltipContent = (
      <Typography variant="body2">No matched priority rules (0 pts)</Typography>
    );
  } else {
    const cleanScore = scoreStr.replace(/^\+/, '');
    tooltipContent = (
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: '1fr auto',
          columnGap: 3,
          rowGap: 0.5,
          alignItems: 'baseline',
        }}
      >
        {matchedIds.map((id) => {
          const rule = rulesById.get(id);
          const expr = rule?.expressionAip160 ?? `Rule #${id}`;
          const weightStr = rule ? formatPoints(rule.weight) : '';
          return (
            <Fragment key={id}>
              <Typography
                variant="body2"
                title={expr}
                sx={{
                  maxWidth: RULE_EXPRESSION_MAX_WIDTH,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {expr}
              </Typography>
              <Typography
                variant="body2"
                sx={{
                  textAlign: 'right',
                  whiteSpace: 'nowrap',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {weightStr}
              </Typography>
            </Fragment>
          );
        })}
        <Divider sx={{ gridColumn: '1 / -1', my: 0.5 }} />
        <Typography
          variant="body2"
          sx={{
            gridColumn: 2,
            textAlign: 'right',
            whiteSpace: 'nowrap',
            fontWeight: 600,
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {`= ${cleanScore} pts`}
        </Typography>
      </Box>
    );
  }

  return (
    <Tooltip
      title={tooltipContent}
      enterDelay={100}
      placement="bottom-start"
      slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
    >
      <Typography
        component="span"
        variant="body2"
        sx={{
          fontSize: '13px',
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
