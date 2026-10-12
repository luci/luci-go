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
  ANDROID_COMMON_DEVICE_FILTERS,
  ANDROID_DEFAULT_COLUMNS,
  BROWSER_COMMON_DEVICE_FILTERS,
  BROWSER_DEFAULT_COLUMNS,
} from './device_config';

describe('device_config', () => {
  it('should export expected default Android columns and common filters', () => {
    expect(ANDROID_DEFAULT_COLUMNS).toContain('id');
    expect(ANDROID_DEFAULT_COLUMNS).toContain('state');

    expect(ANDROID_COMMON_DEVICE_FILTERS).toEqual([
      'state',
      'device_type',
      'build',
      'host_group',
      'model',
      'pool',
      'run_target',
      'type',
    ]);
  });

  it('should export expected default Browser columns and common filters', () => {
    expect(BROWSER_DEFAULT_COLUMNS).toContain('id');

    expect(BROWSER_COMMON_DEVICE_FILTERS).toEqual([
      'ufs.resource_state',
      'ufs.zone',
      'ufs.chrome_platform',
      'sw.device_state',
      'sw.dut_state',
      'sw.pool',
      'realm',
    ]);
  });
});
