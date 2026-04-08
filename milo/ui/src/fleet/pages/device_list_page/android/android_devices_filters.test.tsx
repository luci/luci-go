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

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';

describe('normalizeFilterKey', () => {
  it('should strip labels. prefix and quotes', () => {
    expect(normalizeFilterKey('labels."build"')).toBe('build');
    expect(normalizeFilterKey('labels.build')).toBe('build');
    expect(normalizeFilterKey('build')).toBe('build');
  });

  it('should handle unquoted keys', () => {
    expect(normalizeFilterKey('host_group')).toBe('host_group');
  });
});

describe('Android Devices Table Filtering', () => {
  it('should verify that we can normalize keys consistently', () => {
    const keys = ['labels."build"', 'host_group', 'hostname'];
    const normalized = keys.map(normalizeFilterKey);
    expect(normalized).toEqual(['build', 'host_group', 'hostname']);
  });
});
