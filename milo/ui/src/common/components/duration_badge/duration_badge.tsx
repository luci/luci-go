// Copyright 2023 The LUCI Authors.
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

import { styled, SxProps, Theme } from '@mui/material';
import { scaleThreshold } from 'd3';
import { DateTime, Duration } from 'luxon';
import { useState } from 'react';
import { useInterval } from 'react-use';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { displayCompactDuration } from '@/common/tools/time_utils';

import { DurationTooltip } from './tooltip';

const defaultColorScaleMs = scaleThreshold(
  [
    Duration.fromObject({ seconds: 20 }).toMillis(),
    Duration.fromObject({ minutes: 1 }).toMillis(),
    Duration.fromObject({ minutes: 5 }).toMillis(),
    Duration.fromObject({ minutes: 15 }).toMillis(),
    Duration.fromObject({ hours: 1 }).toMillis(),
    Duration.fromObject({ hours: 3 }).toMillis(),
    Duration.fromObject({ hours: 12 }).toMillis(),
  ],
  [
    {
      backgroundColor: 'hsl(206, 85%, 95%)',
      color: 'var(--light-text-color)',
    },
    {
      backgroundColor: 'hsl(206, 85%, 85%)',
      color: 'var(--light-text-color)',
    },
    { backgroundColor: 'hsl(206, 85%, 75%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 65%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 55%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 45%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 35%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 25%)', color: 'white' },
  ],
);

const defaultColorScale = (d: Duration) => defaultColorScaleMs(d.toMillis());

const Badge = styled('span')`
  color: var(--light-text-color);
  background-color: var(--light-active-color);
  display: inline-block;
  padding: 0.25em 0.4em;
  font-size: 75%;
  font-weight: 500;
  line-height: 13px;
  text-align: center;
  white-space: nowrap;
  vertical-align: middle;
  border-radius: 0.25rem;
  width: 35px;
`;

export interface DurationBadgeProps {
  /**
   * The label for `duration`. Defaults to `"Duration"`.
   */
  readonly durationLabel?: string;
  /**
   * The duration to be rendered. Can be one of the following:
   *  * a `Duration`. Must be non-negative. Renders a short duration string in a
   *    color-scaled badge.
   *  * a `null`. Renders `"N/A"`.
   *
   * Defaults to:
   *  * `from - to` when both `from` and `to` are `DateTime`s, or
   *  * `from - now` when both `from` is a `DateTime`, or
   *  * `now - to` when `to` is a `DateTime`, or
   *  * `null`.
   */
  readonly duration?: Duration | null;
  /**
   * The label for `from`. Defaults to `"From"`.
   */
  readonly fromLabel?: string;
  /**
   * The start time of the duration. Can be one of the following:
   *  * a `DateTime`. Must be less than or equals to `to`. Renders the start
   *    time in the tooltip.
   *  * a `null`, renders `"N/A"`.
   *
   * Defaults to `null`.
   */
  readonly from?: DateTime | null;
  /**
   * The label for `to`. Defaults to `"To"`.
   */
  readonly toLabel?: string;
  /**
   * The end time of the duration. Can be one of the following:
   *  * a `DateTime`. Must be greater than or equals to `from`. Renders the end
   *    time in the tooltip.
   *  * a `null`, renders `"N/A"`.
   *
   * Defaults to `null`.
   */
  readonly to?: DateTime | null;
  /**
   * Update interval in milliseconds. Only used when the duration is calculated
   * from the current timestamp.
   *
   * Defaults to 1 min.
   */
  readonly intervalMs?: number;
  /**
   * Controls the text and background color base on the duration.
   *
   * When not specified, a default color scale is used.
   */
  readonly colorScale?: (duration: Duration) => {
    backgroundColor: string;
    color: string;
  };
  readonly sx?: SxProps<Theme>;
  readonly className?: string;
}

/**
 * Renders a duration badge.
 */
export function DurationBadge({
  durationLabel = 'Duration',
  duration,
  fromLabel = 'From',
  from = null,
  toLabel = 'To',
  to = null,
  intervalMs = 60_000,
  colorScale = defaultColorScale,
  sx,
  className,
}: DurationBadgeProps) {
  const [now, setNow] = useState(() => DateTime.now());
  const shouldUpdate = !duration && Boolean(from) !== Boolean(to);
  useInterval(() => setNow(DateTime.now()), shouldUpdate ? intervalMs : null);

  const calcDuration =
    duration === undefined && (to || from)
      ? (to || now).diff(from || now)
      : duration;

  const [compactDuration] = calcDuration
    ? displayCompactDuration(calcDuration)
    : ['N/A'];

  return (
    <HtmlTooltip
      title={
        <DurationTooltip
          durationLabel={durationLabel}
          duration={calcDuration}
          fromLabel={fromLabel}
          from={from}
          toLabel={toLabel}
          to={to}
        />
      }
      arrow
    >
      <Badge
        sx={{
          ...(duration ? colorScale(duration) : {}),
          ...sx,
        }}
        className={className}
        data-testid="duration"
      >
        {compactDuration}
      </Badge>
    </HtmlTooltip>
  );
}
