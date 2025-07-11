// Copyright 2025 The LUCI Authors.
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
  Device,
  DeviceState,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

/**
 * Gets the state of a DUT from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns String with the state of the DUT.
 */
export const extractDutState = (device?: Device): string => {
  if (
    !device ||
    !device.deviceSpec ||
    !device.deviceSpec.labels ||
    !('dut_state' in device.deviceSpec.labels) ||
    !device.deviceSpec.labels['dut_state'].values.length
  )
    return '';
  return device.deviceSpec.labels['dut_state'].values[0];
};

/**
 * Gets the id of a DUT from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns String with the id of the DUT.
 */
export const extractDutId = (device?: Device | null): string => {
  if (!device) {
    return '';
  }

  if (device.dutId) {
    return device.dutId;
  }

  if (
    !device.deviceSpec ||
    !device.deviceSpec.labels ||
    !('dut_id' in device.deviceSpec.labels) ||
    !device.deviceSpec.labels['dut_id'].values.length
  )
    return '';
  return device.deviceSpec.labels['dut_id'].values[0];
};

/**
 * Gets the string representation from a device's state.
 * @param device Device data type from the Fleet Console backend.
 * @returns Human-readable state string.
 */
export const getDeviceStateString = (device?: Device | undefined): string => {
  return device !== undefined
    ? DeviceState[device.state].replace('DEVICE_STATE_', '')
    : '';
};
