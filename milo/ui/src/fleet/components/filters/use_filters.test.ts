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

import {
  extractActiveFilterFields,
  extractChangedFilterFields,
  formatTrackedFilterFields,
  MAX_GA4_PARAM_LENGTH,
} from './use_filters';

describe('extractActiveFilterFields', () => {
  it('extracts sorted normalized filter keys from AIP-160 expressions', () => {
    expect(
      extractActiveFilterFields(
        'labels."build_type" = "userdebug" AND "run_target" = "oriole" AND labels.sdk_version = "34"',
      ),
    ).toEqual(['build_type', 'run_target', 'sdk_version']);
  });

  it('returns empty array for empty or invalid filters', () => {
    expect(extractActiveFilterFields('')).toEqual([]);
    expect(extractActiveFilterFields(null)).toEqual([]);
    expect(extractActiveFilterFields('invalid === filter')).toEqual([]);
  });
});

describe('extractChangedFilterFields', () => {
  it('returns only added, removed, or modified filter keys', () => {
    const prev =
      'labels."build_type" = "userdebug" AND "run_target" = "pixel-device-1"';
    const next =
      'labels."build_type" = "userdebug" AND "run_target" = "android-board-a" AND labels.sdk_version = "34"';
    expect(extractChangedFilterFields(prev, next)).toEqual([
      'run_target',
      'sdk_version',
    ]);
  });

  it('returns removed keys when a filter is cleared', () => {
    const prev =
      'labels."build_type" = "userdebug" AND "run_target" = "pixel-device-1"';
    const next = 'labels."build_type" = "userdebug"';
    expect(extractChangedFilterFields(prev, next)).toEqual(['run_target']);
  });
});

describe('formatTrackedFilterFields', () => {
  it('caps joined field string at MAX_GA4_PARAM_LENGTH (100 chars)', () => {
    const longFields = Array.from(
      { length: 15 },
      (_, i) => `very_long_android_label_${i}`,
    );
    const formatted = formatTrackedFilterFields(longFields);
    expect(formatted.length).toBe(MAX_GA4_PARAM_LENGTH);
  });
});
