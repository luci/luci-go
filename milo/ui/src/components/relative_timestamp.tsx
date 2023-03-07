// Copyright 2022 The LUCI Authors.
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

import { DateTime, Duration } from 'luxon';
import { useEffect, useState } from 'react';

import { displayDuration } from '../libs/time_utils';

export interface RelativeTimestampProps {
  readonly timestamp: DateTime;
  /**
   * Update interval in ms. Defaults to 1000ms.
   */
  readonly intervalMs?: number;
  readonly formatFn?: (duration: Duration) => string;
}

/**
 * An element that shows the specified duration relative to now.
 */
export function RelativeTimestamp(props: RelativeTimestampProps) {
  const timestamp = props.timestamp;
  const intervalMs = props.intervalMs ?? 1000;
  const formatFn = props.formatFn || displayDuration;

  const [time, setTime] = useState(DateTime.now());

  useEffect(() => {
    const interval = setInterval(() => setTime(DateTime.now()), intervalMs);
    return () => clearInterval(interval);
  }, [intervalMs]);

  const duration = timestamp.diff(time);

  if (duration.toMillis() < 0) {
    return <>{formatFn(duration.negate())} ago</>;
  }
  return <>in {formatFn(duration)}</>;
}
