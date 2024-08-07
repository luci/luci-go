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

import {
  getFormattedDuration,
  getFormattedTimestamp,
} from './timestamp_formatters';

describe('Test timestamp tools', () => {
  const start = '2022-09-06T07:13:12.435231Z';
  const end = '2022-09-06T07:13:16.893998Z';

  test('if duration formatting is correct', () => {
    expect(getFormattedDuration(start, end)).toBe('00:00:04');
  });

  test('if calculating duration is skipped appropriately', () => {
    expect(getFormattedDuration(end, start)).toBe('');
    expect(getFormattedDuration(start, '')).toBe('');
    expect(getFormattedDuration('', end)).toBe('');
  });

  test('if timestamp formatting skips empty strings', () => {
    expect(getFormattedTimestamp('')).toBe('');
  });
});
