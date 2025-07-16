// Copyright 2024 The LUCI Authors.
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

import {
  extractDutId,
  extractDutLabel,
  extractDutState,
  getDeviceStateString,
} from './devices';

const MOCK_DEVICE: Device = {
  // These are required by the Device type, but not relevant for this test.
  id: '',
  dutId: '',
  address: undefined,
  type: 0,
  state: 0,
  deviceSpec: {
    labels: {},
  },
};

describe('devices utils', () => {
  describe('extractDutLabel', () => {
    it('should return the correct label value when it exists', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {
            test_label: { values: ['test_value'] },
          },
        },
      };
      expect(extractDutLabel('test_label', device)).toBe('test_value');
    });

    it('should return an empty string when the label does not exist', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {},
        },
      };
      expect(extractDutLabel('nonexistent_label', device)).toBe('');
    });

    it('should return an empty string when the device or deviceSpec is undefined', () => {
      expect(extractDutLabel('test_label', undefined)).toBe('');
      const deviceWithoutSpec: Device = {
        ...MOCK_DEVICE,
        deviceSpec: undefined,
      };
      expect(extractDutLabel('test_label', deviceWithoutSpec)).toBe('');
    });

    it('should return an empty string when the label values are empty', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {
            empty_label: { values: [] },
          },
        },
      };
      expect(extractDutLabel('empty_label', device)).toBe('');
    });
  });

  describe('extractDutState', () => {
    it('should return the correct DUT state when it exists', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {
            dut_state: { values: ['ready'] },
          },
        },
      };
      expect(extractDutState(device)).toBe('ready');
    });

    it('should return an empty string when the DUT state does not exist', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {},
        },
      };
      expect(extractDutState(device)).toBe('');
    });

    it('should return an empty string when the device or deviceSpec is undefined', () => {
      expect(extractDutState(undefined)).toBe('');
      const deviceWithoutSpec: Device = {
        ...MOCK_DEVICE,
        deviceSpec: undefined,
      };
      expect(extractDutState(deviceWithoutSpec)).toBe('');
    });
  });

  describe('extractDutId', () => {
    it('should return the dutId when it exists', () => {
      const device: Device = { ...MOCK_DEVICE, dutId: '12345' };
      expect(extractDutId(device)).toBe('12345');
    });

    it('should return the dut_id label when dutId is not available', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        deviceSpec: {
          labels: {
            dut_id: { values: ['67890'] },
          },
        },
      };
      expect(extractDutId(device)).toBe('67890');
    });

    it('should return an empty string when neither dutId nor dut_id label exists', () => {
      const device: Device = { ...MOCK_DEVICE, deviceSpec: { labels: {} } };
      expect(extractDutId(device)).toBe('');
    });

    it('should return an empty string when the device is undefined', () => {
      expect(extractDutId(undefined)).toBe('');
    });
  });

  describe('getDeviceStateString', () => {
    it('should return the correct state string', () => {
      const device: Device = {
        ...MOCK_DEVICE,
        state: DeviceState.DEVICE_STATE_AVAILABLE,
      };
      expect(getDeviceStateString(device)).toBe('AVAILABLE');
    });

    it('should return an empty string when the device is undefined', () => {
      expect(getDeviceStateString(undefined)).toBe('');
    });
  });
});
