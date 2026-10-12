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

import { AndroidMachineType } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidMachineTypeLabel, isAndroidHost } from './android_machine_type';

describe('androidMachineTypeLabel', () => {
  it('maps DEVICE to "device"', () => {
    expect(
      androidMachineTypeLabel(AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE),
    ).toBe('device');
  });

  it('maps HOST to "host"', () => {
    expect(
      androidMachineTypeLabel(AndroidMachineType.ANDROID_MACHINE_TYPE_HOST),
    ).toBe('host');
  });

  it('maps UNSPECIFIED to an empty string', () => {
    expect(
      androidMachineTypeLabel(
        AndroidMachineType.ANDROID_MACHINE_TYPE_UNSPECIFIED,
      ),
    ).toBe('');
  });
});

describe('isAndroidHost', () => {
  it('is true only for HOST', () => {
    expect(isAndroidHost(AndroidMachineType.ANDROID_MACHINE_TYPE_HOST)).toBe(
      true,
    );
    expect(isAndroidHost(AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE)).toBe(
      false,
    );
    expect(
      isAndroidHost(AndroidMachineType.ANDROID_MACHINE_TYPE_UNSPECIFIED),
    ).toBe(false);
  });
});
