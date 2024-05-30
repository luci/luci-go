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

import { SxProps, Theme } from '@mui/material';
import { DateTime } from 'luxon';
import { useEffect, useState } from 'react';

import { DurationBadge } from '@/common/components/duration_badge';

// 10s.
export const DEFAULT_TICK_MS = 10000;

interface RelativeDurationBadgeProps {
  readonly from: DateTime;
  /**
   * When not specified, `DateTime.now()` is used, and the `to` will be updated
   * every `tickMs` milliseconds.
   */
  readonly to?: DateTime;
  /**
   * How frequent the duration should update in milliseconds.
   *
   * Defaults to `DEFAULT_TICK_MS`.
   * If `to` is set, `tickMs` is always `Infinity`.
   */
  readonly tickMs?: number;

  readonly sx?: SxProps<Theme>;
  readonly className?: string;
}

/**
 * A variant of `<DurationBadge />` that calculates the duration from the
 * specified start time and end time.
 */
export function RelativeDurationBadge(props: RelativeDurationBadgeProps) {
  const { sx, className } = props;
  const tickMs = props.to ? Infinity : props.tickMs || DEFAULT_TICK_MS;
  const from = props.from;
  const [now, setNow] = useState(() => DateTime.now());
  useEffect(() => {
    if (tickMs === Infinity) {
      return;
    }

    const timer = setInterval(() => setNow(DateTime.now()), tickMs);
    return () => clearInterval(timer);
  }, [tickMs]);

  const duration = (props.to || now).diff(props.from);

  return (
    <DurationBadge
      duration={duration}
      from={from}
      sx={sx}
      className={className}
    />
  );
}
