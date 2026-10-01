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
  CHROMEOS_COMMON_DEVICE_FILTERS,
  CHROMEOS_DEFAULT_COLUMNS,
  generateDutNameRedirectURL,
} from './chromeos_device_config';

describe('chromeos_device_config', () => {
  it('should export expected default ChromeOS columns', () => {
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('dut_id');
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('dut_state');
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('label-board');
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('label-model');
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('label-pool');
    expect(CHROMEOS_DEFAULT_COLUMNS).toContain('label-servo_state');
  });

  it('should export common ChromeOS device filters', () => {
    expect(CHROMEOS_COMMON_DEVICE_FILTERS).toContain('labels."dut_state"');
    expect(CHROMEOS_COMMON_DEVICE_FILTERS).toContain('labels."label-board"');
    expect(CHROMEOS_COMMON_DEVICE_FILTERS).toContain('labels."label-model"');
    expect(CHROMEOS_COMMON_DEVICE_FILTERS).toContain('labels."label-pool"');
    expect(CHROMEOS_COMMON_DEVICE_FILTERS).toContain('labels."label-phase"');
  });

  it('should correctly format generateDutNameRedirectURL', () => {
    const url = generateDutNameRedirectURL('chromeos6-row1-rack2-host3');
    expect(url).toBe(
      '/ui/fleet/redirects/singledevice?filters=labels.%22dut_name%22%20%3D%20%22chromeos6-row1-rack2-host3%22',
    );
  });

  it('should escape AIP special characters before URL-encoding in generateDutNameRedirectURL', () => {
    const url = generateDutNameRedirectURL('host\\"name');
    expect(url).toBe(
      '/ui/fleet/redirects/singledevice?filters=labels.%22dut_name%22%20%3D%20%22host%5C%5C%5C%22name%22',
    );
  });
});
