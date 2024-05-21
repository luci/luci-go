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

import { LONG_TIME_FORMAT } from '@/common/tools/time_utils';
import { DateTime } from 'luxon';

export function getFormattedDuration(start?: string, end?: string): string {
  if (!start || !end) {
    return '';
  }

  const startTime = DateTime.fromISO(start);
  const endTime = DateTime.fromISO(end);

  if (endTime < startTime) {
    return '';
  }

  const diff = endTime.diff(startTime);
  return diff.toFormat('hh:mm:ss');
}

export function getFormattedTimestamp(datetime?: string): string {
  if (!datetime) {
    return '';
  }

  return DateTime.fromISO(datetime).toFormat(LONG_TIME_FORMAT);
}
