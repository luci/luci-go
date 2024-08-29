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

import { DateTime } from 'luxon';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { LONG_TIME_FORMAT } from '@/common/tools/time_utils';

import { DEFAULT_EXTRA_ZONE_CONFIGS, TimeZoneConfig } from './constants';
import { Tooltip } from './tooltip';

export interface TimestampProps {
  readonly datetime: DateTime;
  /**
   * Defaults to `LONG_TIME_FORMAT`;
   */
  readonly format?: string;
  /**
   * Extra timezones to render in the tooltip.
   */
  readonly extraTimezones?: {
    /**
     * Extra timezones to render. Defaults to `DEFAULT_EXTRA_ZONE_CONFIGS`.
     */
    readonly zones?: readonly TimeZoneConfig[];
    /**
     * Defaults to `format`.
     */
    readonly format?: string;
  };
}

/**
 * Renders a timestamp.
 * Shows duration and addition timezone on hover.
 */
export function Timestamp({
  datetime,
  format = LONG_TIME_FORMAT,
  extraTimezones,
}: TimestampProps) {
  const extraZones = extraTimezones?.zones ?? DEFAULT_EXTRA_ZONE_CONFIGS;
  const extraFormat = extraTimezones?.format ?? format;

  return (
    <HtmlTooltip
      title={
        <Tooltip datetime={datetime} format={extraFormat} zones={extraZones} />
      }
    >
      <span>{datetime.toFormat(format)}</span>
    </HtmlTooltip>
  );
}
