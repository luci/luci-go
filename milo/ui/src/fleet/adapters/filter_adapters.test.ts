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

import { healthToDeviceFilter } from './filter_adapters';

describe('healthToDeviceFilter', () => {
  it('returns empty string for empty input', () => {
    expect(healthToDeviceFilter('')).toBe('');
    expect(healthToDeviceFilter(undefined)).toBe('');
    expect(healthToDeviceFilter(null)).toBe('');
  });

  it('remaps model and pool dimensions to label prefixes', () => {
    expect(healthToDeviceFilter('model = "brya"')).toBe(
      'labels.model = "brya"',
    );
    expect(healthToDeviceFilter('pool = "DUT_POOL_QUOTA"')).toBe(
      'labels.pool = "DUT_POOL_QUOTA"',
    );
    expect(healthToDeviceFilter('device_id = "dut-01"')).toBe('id = "dut-01"');
  });

  it('preserves existing labels and compound predicates', () => {
    const input =
      '(model = "brya" OR model = "corsola") AND labels.board = "volteer"';
    expect(healthToDeviceFilter(input)).toBe(
      '(labels.model = "brya" OR labels.model = "corsola") AND labels.board = "volteer"',
    );
  });
});
