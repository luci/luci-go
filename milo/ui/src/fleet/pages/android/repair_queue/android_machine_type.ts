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

/**
 * Human readable label for an `AndroidMachineType`. Matches the values of the
 * `android_devices.fc_machine_type` column (`device` / `host`) so it can be
 * shown next to filters that target that column. Returns an empty string for
 * `ANDROID_MACHINE_TYPE_UNSPECIFIED`.
 */
export function androidMachineTypeLabel(t: AndroidMachineType): string {
  switch (t) {
    case AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE:
      return 'device';
    case AndroidMachineType.ANDROID_MACHINE_TYPE_HOST:
      return 'host';
    default:
      return '';
  }
}

/** Whether the repair queue row refers to a host rather than a device. */
export function isAndroidHost(t: AndroidMachineType): boolean {
  return t === AndroidMachineType.ANDROID_MACHINE_TYPE_HOST;
}
