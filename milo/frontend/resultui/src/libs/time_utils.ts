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

import { DateTime, Duration } from 'luxon';

import { Timestamp } from '../services/buildbucket';

export const DEFAULT_TIME_FORMAT = 'ccc, HH:mm:ss MMM dd yyyy ZZZZ';

export function displayTimestamp(t: Timestamp): string {
  const d = datetimeFromTimestamp(t);
  return d.toFormat(DEFAULT_TIME_FORMAT);
}

export function displayTimestampOpt(t?: Timestamp): string | null {
  if (!t) {
    return null;
  }
  return displayTimestamp(t);
}

export function displayTimeDiff(beginTime: Timestamp, endTime: Timestamp): string {
  const bd = datetimeFromTimestamp(beginTime);
  const ed = datetimeFromTimestamp(endTime);
  return displayDuration(ed.diff(bd));
}

export function displayTimeDiffOpt(beginTime?: Timestamp, endTime?: Timestamp): string | null {
  if (!beginTime || !endTime) {
    return null;
  }
  return displayTimeDiff(beginTime, endTime);
}

export function displayDuration(duration: Duration) {
  const shifted = duration.shiftTo('days', 'hours', 'minutes', 'seconds', 'milliseconds');
  const parts = [];
  if (shifted.days >= 1) {
    parts.push(shifted.days + ' ' + (shifted.days === 1 ? 'day' : 'days'));
  }
  if (shifted.hours >= 1) {
    parts.push(shifted.hours + ' ' + (shifted.hours === 1 ? 'hour' : 'hours'));
  }
  // We only care about minutes if days and hours are not both present
  if (shifted.minutes >= 1 && parts.length <= 1) {
    parts.push(shifted.minutes + ' ' + (shifted.minutes === 1 ? 'min' : 'mins'));
  }
  // We only care about seconds if it is significant enough
  if (shifted.seconds >= 1 && parts.length <= 1) {
    parts.push(shifted.seconds + ' ' + (shifted.seconds === 1 ? 'sec' : 'secs'));
  }
  // We only care about ms if there are no other part
  if (shifted.milliseconds > 0 && parts.length === 0) {
    parts.push(Math.floor(shifted.milliseconds) + ' ms');
  }
  return parts.join(' ');
}

export function datetimeFromTimestamp(t: Timestamp): DateTime {
  return DateTime.fromMillis(t.seconds * 1000 + t.nanos / 1000000);
}
