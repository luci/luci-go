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
 * Gets all values of a label from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns Array of strings with the label values.
 */
export const extractDutLabels = (
  label: string,
  device?: Device,
): readonly string[] => {
  if (
    !device?.deviceSpec?.labels ||
    !(label in device.deviceSpec.labels) ||
    !device.deviceSpec.labels[label]?.values?.length
  )
    return [];
  return device.deviceSpec.labels[label].values;
};

/**
 * Gets the state of a DUT from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns String with the state of the DUT.
 */
export const extractDutLabel = (label: string, device?: Device): string => {
  if (
    !device?.deviceSpec?.labels ||
    !(label in device.deviceSpec.labels) ||
    !device.deviceSpec.labels[label]?.values?.length
  )
    return '';
  return device.deviceSpec.labels[label].values[0];
};

/**
 * Gets the state of a DUT from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns String with the state of the DUT.
 */
export const extractDutState = (device?: Device): string => {
  return extractDutLabel('dut_state', device);
};

/**
 * Gets the id of a DUT from a device object if it exists.
 * @param device Device data type from the Fleet Console backend.
 * @returns String with the id of the DUT.
 */
export const extractDutId = (device?: Device): string => {
  if (device?.dutId) return device?.dutId;
  return extractDutLabel('dut_id', device);
};

/**
 * Gets the string representation from a device's state.
 * @param device Device data type from the Fleet Console backend.
 * @returns Human-readable state string.
 */
export const getDeviceStateString = (device?: Device): string => {
  if (device === undefined || DeviceState[device.state] === undefined) {
    return '';
  }

  return DeviceState[device.state].replace('DEVICE_STATE_', '');
};

/**
 * Checks if the device is in the partner namespace.
 * @param namespace The namespace of the device.
 * @returns True if the device is in the partner namespace.
 */
export const isPartnerNamespace = (
  namespace?: string | readonly string[],
): boolean => {
  return namespace?.includes('os-partner') ?? false;
};
