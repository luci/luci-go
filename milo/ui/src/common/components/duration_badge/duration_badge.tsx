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

import { Interpolation, Theme, css } from '@emotion/react';
import { scaleThreshold } from 'd3';
import { html, render } from 'lit';
import { DateTime, Duration } from 'luxon';

import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
} from '@/common/components/tooltip';
import {
  LONG_TIME_FORMAT,
  displayCompactDuration,
  displayDuration,
} from '@/common/tools/time_utils';

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
  ]
);

const defaultColorScale = (d: Duration) => defaultColorScaleMs(d.toMillis());

const durationBadge = css`
  color: var(--light-text-color);
  background-color: var(--light-active-color);
  display: inline-block;
  padding: 0.25em 0.4em;
  font-size: 75%;
  font-weight: 500;
  line-height: 13px;
  text-align: center;
  white-space: nowrap;
  vertical-align: bottom;
  border-radius: 0.25rem;
  margin-bottom: 3px;
  width: 35px;
`;

function renderTooltip(
  duration: Duration,
  from?: DateTime | null,
  to?: DateTime | null
) {
  return html`
    <table>
      <tr>
        <td>Duration:</td>
        <td>${displayDuration(duration)}</td>
      </tr>
      <tr>
        <td>From:</td>
        <td>${from ? from.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
      </tr>
      <tr>
        <td>To:</td>
        <td>${to ? to.toFormat(LONG_TIME_FORMAT) : 'N/A'}</td>
      </tr>
    </table>
  `;
}

interface DurationBadgeProps {
  readonly duration: Duration;
  /**
   * When specified, renders start time in the tooltip.
   */
  readonly from?: DateTime | null;
  /**
   * When specified, renders end time in the tooltip.
   */
  readonly to?: DateTime | null;
  /**
   * Controls the text and background color base on the duration.
   */
  readonly colorScale?: (duration: Duration) => {
    backgroundColor: string;
    color: string;
  };

  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

/**
 * Renders a duration badge.
 */
export function DurationBadge({
  duration,
  from,
  to,
  colorScale = defaultColorScale,
  css,
  className,
}: DurationBadgeProps) {
  function onShowTooltip(target: HTMLElement) {
    const tooltip = document.createElement('div');
    render(renderTooltip(duration, from, to), tooltip);

    window.dispatchEvent(
      new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
        detail: {
          tooltip,
          targetRect: target.getBoundingClientRect(),
          gapSize: 2,
        },
      })
    );
  }

  function onHideTooltip() {
    window.dispatchEvent(
      new CustomEvent<HideTooltipEventDetail>('hide-tooltip', {
        detail: { delay: 50 },
      })
    );
  }

  const [compactDuration] = displayCompactDuration(duration);

  return (
    <span
      css={[durationBadge, css, colorScale(duration)]}
      className={className}
      onMouseOver={(e) => onShowTooltip(e.target as HTMLElement)}
      onFocus={(e) => onShowTooltip(e.target as HTMLElement)}
      onMouseOut={onHideTooltip}
      onBlur={onHideTooltip}
      data-testid="duration"
    >
      {compactDuration}
    </span>
  );
}
