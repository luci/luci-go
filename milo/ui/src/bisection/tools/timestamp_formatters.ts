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

import dayjs from 'dayjs';
import advancedFormat from 'dayjs/plugin/advancedFormat';
import duration from 'dayjs/plugin/duration';
import timezone from 'dayjs/plugin/timezone';
import utc from 'dayjs/plugin/utc';

dayjs.extend(duration);
dayjs.extend(utc);
dayjs.extend(timezone);
dayjs.extend(advancedFormat);

export function getFormattedDuration(start: string, end: string): string {
  if (!start || !end) {
    return '';
  }

  const startTime = dayjs(start);
  const endTime = dayjs(end);

  if (endTime < startTime) {
    return '';
  }

  const diff = dayjs.duration(endTime.diff(startTime));
  return diff.format('HH:mm:ss');
}

export function getFormattedTimestamp(datetime: string): string {
  if (!datetime) {
    return '';
  }

  return dayjs(datetime).format('HH:mm:ss ddd, MMM DD YYYY z');
}
