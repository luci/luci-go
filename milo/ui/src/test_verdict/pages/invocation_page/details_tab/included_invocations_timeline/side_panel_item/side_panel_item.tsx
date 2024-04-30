// Copyright 2024 The LUCI Authors.
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

import { Box, CircularProgress, Link, useTheme } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router-dom';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

import {
  SIDE_PANEL_PADDING,
  SIDE_PANEL_RECT_PADDING,
  SIDE_PANEL_WIDTH,
  TEXT_FONT_SIZE,
  TIME_SPAN_HEIGHT,
} from '../constants';

import { InvocationTooltip } from './tooltip';

export interface SidePanelItemProps {
  readonly invName: string;
  readonly parentCreateTime: DateTime;
}

export function SidePanelItem({
  invName,
  parentCreateTime,
}: SidePanelItemProps) {
  const theme = useTheme();
  const invId = invName.slice('invocations/'.length);

  const client = useResultDbClient();
  const {
    data: invocation,
    isLoading,
    isError,
    error,
  } = useQuery(client.GetInvocation.query({ name: invName }));
  if (isError) {
    throw error;
  }

  return (
    <>
      <rect
        x={SIDE_PANEL_PADDING}
        y={-TIME_SPAN_HEIGHT / 2}
        width={SIDE_PANEL_WIDTH - 2 * SIDE_PANEL_PADDING}
        height={TIME_SPAN_HEIGHT}
        fill="var(--started-bg-color)"
        stroke="var(--started-color)"
      />
      <foreignObject
        x={SIDE_PANEL_PADDING}
        y={-TEXT_FONT_SIZE / 2}
        width={SIDE_PANEL_WIDTH - 2 * SIDE_PANEL_PADDING}
        height={TEXT_FONT_SIZE}
      >
        <HtmlTooltip
          arrow
          disableInteractive
          title={
            <InvocationTooltip
              invId={invId}
              invocation={invocation}
              parentCreateTime={parentCreateTime}
            />
          }
        >
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: '1fr auto',
              padding: `0 ${SIDE_PANEL_RECT_PADDING}px`,
            }}
          >
            <Box
              sx={{
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              <Link
                component={RouterLink}
                to={`/ui/inv/${invId}`}
                sx={{
                  color: theme.palette.text.primary,
                  textDecorationColor: theme.palette.text.primary,
                }}
              >
                {invId}
              </Link>
            </Box>
            {isLoading && <CircularProgress size={TEXT_FONT_SIZE} />}
          </Box>
        </HtmlTooltip>
      </foreignObject>
    </>
  );
}
