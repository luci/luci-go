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

import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getMobileHarnessLink } from './mobile_harness';

describe('getMobileHarnessLink', () => {
  it('returns empty string when device is undefined or null', () => {
    expect(getMobileHarnessLink(undefined)).toBe('');
    expect(getMobileHarnessLink(null)).toBe('');
  });

  describe('host machines', () => {
    it('returns lab detail view when hostname and host_ip are present', () => {
      const device: AndroidDevice = {
        id: 'host-1',
        omnilabSpec: {
          labels: {
            fc_machine_type: { values: ['host'] },
            hostname: { values: ['my-host.domain.com'] },
            host_ip: { values: ['192.168.1.10'] },
          },
        },
      } as unknown as AndroidDevice;

      expect(getMobileHarnessLink(device)).toBe(
        'https://mobileharness-fe.corp.google.com/labdetailview/my-host.domain.com/192.168.1.10',
      );
    });

    it('falls back to host_name label when hostname is absent', () => {
      const device: AndroidDevice = {
        id: 'host-2',
        omnilabSpec: {
          labels: {
            fc_machine_type: { values: ['host'] },
            host_name: { values: ['fallback-host.domain.com'] },
            host_ip: { values: ['10.0.0.1'] },
          },
        },
      } as unknown as AndroidDevice;

      expect(getMobileHarnessLink(device)).toBe(
        'https://mobileharness-fe.corp.google.com/labdetailview/fallback-host.domain.com/10.0.0.1',
      );
    });

    it('returns lab list view filtered by host_name when host_ip is missing', () => {
      const device: AndroidDevice = {
        id: 'host-no-ip',
        omnilabSpec: {
          labels: {
            fc_machine_type: { values: ['host'] },
            hostname: { values: ['my-host'] },
          },
        },
      } as unknown as AndroidDevice;

      const link = getMobileHarnessLink(device);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/lablistview?filter=%22host_name%22%3A%28%22host-no-ip%22%29',
      );
    });

    it('returns lab list view filtered by host_name when hostname is missing', () => {
      const device: AndroidDevice = {
        id: 'host-no-name',
        omnilabSpec: {
          labels: {
            fc_machine_type: { values: ['host'] },
            host_ip: { values: ['10.0.0.1'] },
          },
        },
      } as unknown as AndroidDevice;

      const link = getMobileHarnessLink(device);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/lablistview?filter=%22host_name%22%3A%28%22host-no-name%22%29',
      );
    });
  });

  describe('regular devices', () => {
    it('returns device detail view when hostname and host_ip are present', () => {
      const device: AndroidDevice = {
        id: 'dut-12345',
        omnilabSpec: {
          labels: {
            hostname: { values: ['dut-host.domain.com'] },
            host_ip: { values: ['172.16.0.5'] },
          },
        },
      } as unknown as AndroidDevice;

      expect(getMobileHarnessLink(device)).toBe(
        'https://mobileharness-fe.corp.google.com/devicedetailview/dut-host.domain.com/172.16.0.5/dut-12345',
      );
    });

    it('falls back to host_name label when hostname is absent', () => {
      const device: AndroidDevice = {
        id: 'dut-67890',
        omnilabSpec: {
          labels: {
            host_name: { values: ['alt-host.domain.com'] },
            host_ip: { values: ['172.16.0.6'] },
          },
        },
      } as unknown as AndroidDevice;

      expect(getMobileHarnessLink(device)).toBe(
        'https://mobileharness-fe.corp.google.com/devicedetailview/alt-host.domain.com/172.16.0.6/dut-67890',
      );
    });

    it('returns device list view filtered by id when host_ip is missing', () => {
      const device: AndroidDevice = {
        id: 'dut-no-ip',
        omnilabSpec: {
          labels: {
            hostname: { values: ['dut-host'] },
          },
        },
      } as unknown as AndroidDevice;

      const link = getMobileHarnessLink(device);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/devicelistview?filter=%22id%22%3A%28%22dut-no-ip%22%29',
      );
    });

    it('returns device list view filtered by id when hostname is missing', () => {
      const device: AndroidDevice = {
        id: 'dut-no-name',
        omnilabSpec: {
          labels: {
            host_ip: { values: ['172.16.0.5'] },
          },
        },
      } as unknown as AndroidDevice;

      const link = getMobileHarnessLink(device);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/devicelistview?filter=%22id%22%3A%28%22dut-no-name%22%29',
      );
    });

    it('returns device list view filtered by id when omnilabSpec is empty or undefined', () => {
      const device: AndroidDevice = {
        id: 'dut-empty',
      } as unknown as AndroidDevice;

      const link = getMobileHarnessLink(device);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/devicelistview?filter=%22id%22%3A%28%22dut-empty%22%29',
      );
    });
  });
});
