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

import { Typography } from '@mui/material';
import { DateTime } from 'luxon';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';

export interface SmartRelativeTimestampProps {
  /**
   * The date to be displayed.
   */
  readonly date: DateTime;
}

/**
 * A component that displays a relative timestamp (e.g., "2 days ago") and shows
 * a tooltip with the full timestamp in both the user's local timezone and UTC
 * upon hover.
 */
export function SmartRelativeTimestamp({ date }: SmartRelativeTimestampProps) {
  if (!date.isValid) {
    return null;
  }

  return (
    <HtmlTooltip
      title={
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'auto auto',
            gap: '4px 12px',
            alignItems: 'baseline',
          }}
        >
          <Typography color="inherit" variant="body2">
            Local:
          </Typography>
          <Typography color="inherit" variant="body2">
            {date.toLocaleString(DateTime.DATETIME_FULL)}
          </Typography>

          <Typography color="inherit" variant="body2">
            UTC:
          </Typography>
          <Typography color="inherit" variant="body2">
            {date.toUTC().toLocaleString(DateTime.DATETIME_FULL)}
          </Typography>
        </div>
      }
    >
      <span
        style={{
          cursor: 'help',
          textDecoration: 'underline',
          textDecorationStyle: 'dotted',
          textUnderlineOffset: '3px',
        }}
      >
        <RelativeTimestamp timestamp={date} formatFn={displayApproxDuration} />
      </span>
    </HtmlTooltip>
  );
}
