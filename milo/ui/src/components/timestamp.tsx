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

import { html, render } from 'lit';
import { DateTime } from 'luxon';

import { displayDuration, LONG_TIME_FORMAT } from '../libs/time_utils';
import { HideTooltipEventDetail, ShowTooltipEventDetail } from './tooltip';

export interface TimeZoneConfig {
  label: string;
  zone: string;
}

export const DEFAULT_EXTRA_ZONE_CONFIGS = [
  {
    label: 'PT',
    zone: 'America/Los_Angeles',
  },
  {
    label: 'UTC',
    zone: 'utc',
  },
];

function renderTooltip(
  datetime: DateTime,
  format: string,
  extraZones: readonly TimeZoneConfig[]
) {
  const now = DateTime.now();
  return html`
    <table>
      <tr>
        <td colspan="2">${displayDuration(now.diff(datetime))} ago</td>
      </tr>
      ${extraZones.map(
        (tz) => html`
          <tr>
            <td>${tz.label}:</td>
            <td>${datetime.setZone(tz.zone).toFormat(format)}</td>
          </tr>
        `
      )}
    </table>
  `;
}

export interface TimestampProps {
  readonly datetime: DateTime;
  readonly format?: string;
  readonly extraZones?: readonly TimeZoneConfig[];
}

/**
 * Renders a timestamp.
 * Shows duration and addition timezone on hover.
 */
export function Timestamp(props: TimestampProps) {
  const datetime = props.datetime;
  const format = props.format ?? LONG_TIME_FORMAT;
  const extraZones = props.extraZones ?? DEFAULT_EXTRA_ZONE_CONFIGS;

  function onShowTooltip(target: HTMLElement) {
    const tooltip = document.createElement('div');
    render(renderTooltip(datetime, format, extraZones), tooltip);

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

  return (
    <span
      onMouseOver={(e) => onShowTooltip(e.target as HTMLElement)}
      onFocus={(e) => onShowTooltip(e.target as HTMLElement)}
      onMouseOut={onHideTooltip}
      onBlur={onHideTooltip}
    >
      {datetime.toFormat(format)}
    </span>
  );
}
