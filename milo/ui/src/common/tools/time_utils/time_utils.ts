// Copyright 2020 The LUCI Authors.
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

import { Duration } from 'luxon';

import { Duration as ProtoDuration } from '@/proto/google/protobuf/duration.pb';

export const SHORT_TIME_FORMAT = 'y-MM-dd HH:mm';
export const LONG_TIME_FORMAT = 'HH:mm:ss ccc, MMM dd yyyy ZZZZ';
export const NUMERIC_TIME_FORMAT = 'y-MM-dd HH:mm:ss ZZ';
export const NUMERIC_TIME_FORMAT_WITH_MS = 'y-MM-dd HH:mm:ss.SSS';

/**
 * Displays a non-negative duration.
 *
 * Negative durations are treated as 0.
 */
// The long format is incompatible with negative duration.
// The caller should decide how to handle negative duration (e.g. add a `" ago"`
// suffix, or add a `"-"` prefix).
export function displayDuration(duration: Duration) {
  const durationMs = duration.toMillis();
  if (durationMs < 0) {
    duration = Duration.fromMillis(0);
  }

  const shifted = duration.shiftTo(
    'days',
    'hours',
    'minutes',
    'seconds',
    'milliseconds',
  );
  const parts = [];
  if (shifted.days >= 1) {
    parts.push(shifted.days + ' ' + (shifted.days === 1 ? 'day' : 'days'));
  }
  if (shifted.hours >= 1) {
    parts.push(shifted.hours + ' ' + (shifted.hours === 1 ? 'hour' : 'hours'));
  }
  // We only care about minutes if days and hours are not both present
  if (shifted.minutes >= 1 && parts.length <= 1) {
    parts.push(
      shifted.minutes + ' ' + (shifted.minutes === 1 ? 'min' : 'mins'),
    );
  }
  // We only care about seconds if it is significant enough
  if (shifted.seconds >= 1 && parts.length <= 1) {
    parts.push(
      shifted.seconds + ' ' + (shifted.seconds === 1 ? 'sec' : 'secs'),
    );
  }
  // We only care about ms if there are no other part
  if (parts.length === 0) {
    parts.push(Math.floor(shifted.milliseconds) + ' ms');
  }
  return parts.join(' ');
}

/**
 * Displays a non-negative duration on a compact format.
 *
 * Negative durations are treated as 0.
 */
// We can add support for negative duration but adding a sign while not
// increasing the length of the output string can make the implementation
// complicated and the output precision unpredictable.
export function displayCompactDuration(
  duration: Duration | null,
): [string, string] {
  if (duration === null) {
    return ['N/A', ''];
  }

  const durationMs = duration.toMillis();
  if (durationMs < 0) {
    duration = Duration.fromMillis(0);
  }

  const shifted = duration.shiftTo(
    'days',
    'hours',
    'minutes',
    'seconds',
    'milliseconds',
  );
  if (shifted.days >= 1) {
    return [`${(shifted.days + shifted.hours / 24).toFixed(1)}d`, 'd'];
  }
  if (shifted.hours >= 1) {
    return [`${(shifted.hours + shifted.minutes / 60).toPrecision(2)}h`, 'h'];
  }
  if (shifted.minutes >= 1) {
    return [`${(shifted.minutes + shifted.seconds / 60).toPrecision(2)}m`, 'm'];
  }

  // Unlike other units, shifted.milliseconds may not be an integer.
  // shifted.milliseconds.toFixed(0) may give us 1000 when
  // shifted.milliseconds >= 999.5. We should display it as 1.0s in that case.
  if (shifted.seconds >= 1 || shifted.milliseconds >= 999.5) {
    return [
      `${(shifted.seconds + shifted.milliseconds / 1000).toPrecision(2)}s`,
      's',
    ];
  }
  return [`${shifted.milliseconds.toFixed(0)}ms`, 'ms'];
}

/* eslint-disable max-len */
/**
 * Parses the JSON encoding of google.protobuf.Duration (e.g. '4.5s').
 * https://github.com/protocolbuffers/protobuf/blob/68cb69ea68822d96eee6d6104463edf85e70d689/src/google/protobuf/duration.proto#L92
 *
 * Returns the number of milliseconds.
 */
/* eslint-disable max-len */
export function parseProtoDurationStr(duration: string): Duration {
  return Duration.fromObject({
    second: Number(duration.substring(0, duration.length - 1)),
  });
}

/**
 * Converts a google.protobuf.Duration object to a luxon Duration object.
 */
export function parseProtoDuration(duration: ProtoDuration): Duration {
  return Duration.fromObject({
    second: Number(duration.seconds),
    millisecond: duration.nanos / 1000000,
  });
}
