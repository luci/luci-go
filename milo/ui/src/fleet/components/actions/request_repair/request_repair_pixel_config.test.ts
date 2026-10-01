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
  generateDeviceDetailsURL,
  PIXEL_PLATFORM,
} from '@/fleet/constants/paths';
import { getMobileHarnessLink } from '@/fleet/utils/mobile_harness';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { HealthCategory } from '@/proto/go.chromium.org/infra/fleetconsole/internal/infra/ext/omnilab/omnilab-device.pb';

import {
  areDevicesFromSingleLab,
  extractPixelLabName,
  generatePixelDeviceDetails,
  generatePixelDevicesTable,
  generatePixelRestorationDescription,
  generatePixelRestorationTitle,
  PixelRestorationConfig,
  PIXEL_RESTORATION_COMPONENT_ID,
  PIXEL_RESTORATION_TEMPLATE_ID,
} from './request_repair_pixel_config';

const createMockDevice = (
  overrides?: Partial<AndroidDevice>,
): AndroidDevice => ({
  id: 'device-001',
  runTarget: 'oriole',
  realm: 'pixel-realm',
  healthCategory: HealthCategory.HEALTH_CATEGORY_IN_SERVICE,
  fcOfflineSince: '2026-09-29T10:00:00Z',
  omnilabSpec: {
    labels: {
      lab_name: { values: ['mock-lab-mtv'] },
      hostname: { values: ['host-001.mock-lab-mtv'] },
      host_ip: { values: ['10.0.0.1'] },
      dut_state: { values: ['ready'] },
      fc_is_offline: { values: ['false'] },
      fc_machine_type: { values: ['device'] },
      location_tag: { values: ['rack-1-shelf-2'] },
    },
  },
  ...overrides,
});

describe('request_repair_pixel_config', () => {
  describe('extractPixelLabName', () => {
    it('extracts lab_name when present', () => {
      const dev = createMockDevice();
      expect(extractPixelLabName(dev)).toBe('mock-lab-mtv');
    });

    it('falls back to lab_location when lab_name is missing', () => {
      const dev = createMockDevice({
        omnilabSpec: {
          labels: {
            lab_location: { values: ['fallback-lab'] },
          },
        },
      });
      expect(extractPixelLabName(dev)).toBe('fallback-lab');
    });

    it('returns empty string when both lab_name and lab_location are missing', () => {
      const dev = createMockDevice({
        omnilabSpec: { labels: {} },
      });
      expect(extractPixelLabName(dev)).toBe('');
    });

    it('trims whitespace and returns empty string if only whitespace', () => {
      const dev = createMockDevice({
        omnilabSpec: {
          labels: {
            lab_name: { values: ['   '] },
          },
        },
      });
      expect(extractPixelLabName(dev)).toBe('');
    });
  });

  describe('areDevicesFromSingleLab', () => {
    it('returns false for an empty list', () => {
      expect(areDevicesFromSingleLab([])).toBe(false);
    });

    it('returns true for a single device with valid lab', () => {
      expect(areDevicesFromSingleLab([createMockDevice()])).toBe(true);
    });

    it('returns false for a single device with no lab', () => {
      const dev = createMockDevice({ omnilabSpec: { labels: {} } });
      expect(areDevicesFromSingleLab([dev])).toBe(false);
    });

    it('returns true when all devices share the same lab', () => {
      const dev1 = createMockDevice({ id: 'dev-1' });
      const dev2 = createMockDevice({ id: 'dev-2' });
      expect(areDevicesFromSingleLab([dev1, dev2])).toBe(true);
    });

    it('returns false when devices have different labs', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: { lab_name: { values: ['lab-a'] } } },
      });
      const dev2 = createMockDevice({
        id: 'dev-2',
        omnilabSpec: { labels: { lab_name: { values: ['lab-b'] } } },
      });
      expect(areDevicesFromSingleLab([dev1, dev2])).toBe(false);
    });

    it('returns false when one device has lab and another has missing lab', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: { lab_name: { values: ['lab-a'] } } },
      });
      const dev2 = createMockDevice({
        id: 'dev-2',
        omnilabSpec: { labels: {} },
      });
      expect(areDevicesFromSingleLab([dev1, dev2])).toBe(false);
    });

    it('returns false when all devices have missing labs (does not group unknown labs)', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: {} },
      });
      const dev2 = createMockDevice({
        id: 'dev-2',
        omnilabSpec: { labels: {} },
      });
      expect(areDevicesFromSingleLab([dev1, dev2])).toBe(false);
    });
  });

  describe('generatePixelRestorationTitle', () => {
    it('throws error when devices array is empty', () => {
      expect(() => generatePixelRestorationTitle([])).toThrow(
        'No devices specified',
      );
    });

    it('throws error when devices have no known lab', () => {
      const dev = createMockDevice({ omnilabSpec: { labels: {} } });
      expect(() => generatePixelRestorationTitle([dev])).toThrow(
        'Selected devices must belong to the same lab',
      );
    });

    it('throws error when devices are from different labs', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: { lab_name: { values: ['lab-a'] } } },
      });
      const dev2 = createMockDevice({
        id: 'dev-2',
        omnilabSpec: { labels: { lab_name: { values: ['lab-b'] } } },
      });
      expect(() => generatePixelRestorationTitle([dev1, dev2])).toThrow(
        'Selected devices must belong to the same lab',
      );
    });

    it('generates title for a single device', () => {
      const dev = createMockDevice({ id: 'pixel-123' });
      expect(generatePixelRestorationTitle([dev])).toBe(
        '[mock-lab-mtv][Restoration][pixel-123]',
      );
    });

    it('generates title for two devices', () => {
      const dev1 = createMockDevice({ id: 'pixel-1' });
      const dev2 = createMockDevice({ id: 'pixel-2' });
      expect(generatePixelRestorationTitle([dev1, dev2])).toBe(
        '[mock-lab-mtv][Restoration][pixel-1, pixel-2]',
      );
    });

    it('generates title for three or more devices', () => {
      const dev1 = createMockDevice({ id: 'pixel-1' });
      const dev2 = createMockDevice({ id: 'pixel-2' });
      const dev3 = createMockDevice({ id: 'pixel-3' });
      expect(generatePixelRestorationTitle([dev1, dev2, dev3])).toBe(
        '[mock-lab-mtv][Restoration] [3] - [Multiple Devices]',
      );
    });
  });

  describe('generatePixelDeviceDetails', () => {
    it('formats details for a single device including all metadata and links', () => {
      const dev = createMockDevice();
      const details = generatePixelDeviceDetails(dev);

      expect(details).toContain('* **Device:** [device\\-001]');
      expect(details).toContain(
        generateDeviceDetailsURL(PIXEL_PLATFORM, 'device-001'),
      );
      expect(details).toContain('**Hostname:** host\\-001\\.mock\\-lab\\-mtv');
      expect(details).toContain('**Host IP:** 10\\.0\\.0\\.1');
      expect(details).toContain('**Run Target:** oriole');
      expect(details).toContain('**State:** ready');
      expect(details).toContain('**Offline:** false');
      expect(details).toContain('**Offline Since:** 2026\\-09\\-29T10:00:00Z');
      expect(details).toContain('**Location Tag:** rack\\-1\\-shelf\\-2');
      expect(details).toContain('**Mobile Harness:** [View in Mobile Harness]');
    });

    it('falls back to product_board or hardware when runTarget is empty', () => {
      const dev = createMockDevice({
        runTarget: '',
        omnilabSpec: {
          labels: {
            product_board: { values: ['testboard'] },
          },
        },
      });
      const details = generatePixelDeviceDetails(dev);
      expect(details).toContain('**Run Target:** testboard');
    });

    it('handles mobile harness link for host machines', () => {
      const hostDev = createMockDevice({
        id: 'host-machine-1',
        omnilabSpec: {
          labels: {
            fc_machine_type: { values: ['host'] },
            hostname: { values: ['my-host'] },
            host_ip: { values: ['10.1.1.1'] },
          },
        },
      });
      const link = getMobileHarnessLink(hostDev);
      expect(link).toBe(
        'https://mobileharness-fe.corp.google.com/labdetailview/my-host/10.1.1.1',
      );
    });

    it('handles mobile harness link for devices without hostname/host_ip', () => {
      const dev = createMockDevice({
        id: 'dev-no-host',
        omnilabSpec: { labels: {} },
      });
      const link = getMobileHarnessLink(dev);
      expect(link).toContain(
        'https://mobileharness-fe.corp.google.com/devicelistview',
      );
      expect(link).toContain('dev-no-host');
    });
  });

  describe('generatePixelDevicesTable', () => {
    it('generates a markdown table for devices with links', () => {
      const dev1 = createMockDevice({ id: 'dev-1' });
      const dev2 = createMockDevice({ id: 'dev-2' });
      const table = generatePixelDevicesTable([dev1, dev2], true);

      expect(table).toContain(
        '| Device ID | Hostname | Run Target | State | Mobile Harness |',
      );
      expect(table).toContain('| --- | --- | --- | --- | --- |');
      expect(table).toContain('[dev\\-1]');
      expect(table).toContain('[dev\\-2]');
      expect(table).toContain('[View in MH](');
      expect(table).toContain(getMobileHarnessLink(dev1));
      expect(table).toContain(getMobileHarnessLink(dev2));
    });

    it('generates a compact markdown table omitting device links when includeLinks is false', () => {
      const dev1 = createMockDevice({ id: 'dev-1' });
      const dev2 = createMockDevice({ id: 'dev-2' });
      const table = generatePixelDevicesTable([dev1, dev2], false);

      expect(table).toContain(
        '| Device ID | Hostname | Run Target | State | Mobile Harness |',
      );
      expect(table).not.toContain('[dev\\-1](');
      expect(table).toContain('[View in MH](');
      expect(table).toContain(getMobileHarnessLink(dev1));
      expect(table).toContain(getMobileHarnessLink(dev2));
      expect(table).toContain(
        '| dev\\-1 | host\\-001\\.mock\\-lab\\-mtv | oriole | ready | [View in MH](',
      );
    });
  });

  describe('generatePixelRestorationDescription', () => {
    it('throws error when devices array is empty', () => {
      expect(() => generatePixelRestorationDescription([])).toThrow(
        'No devices specified',
      );
    });

    it('throws error when devices come from multiple labs', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: { lab_name: { values: ['lab-a'] } } },
      });
      const dev2 = createMockDevice({
        id: 'dev-2',
        omnilabSpec: { labels: { lab_name: { values: ['lab-b'] } } },
      });
      expect(() => generatePixelRestorationDescription([dev1, dev2])).toThrow(
        'Selected devices must belong to the same lab',
      );
    });

    it('throws error when devices have no known lab', () => {
      const dev1 = createMockDevice({
        id: 'dev-1',
        omnilabSpec: { labels: {} },
      });
      expect(() => generatePixelRestorationDescription([dev1])).toThrow(
        'Selected devices must belong to the same lab',
      );
    });

    it('places the lab name at the very top of the description', () => {
      const dev = createMockDevice();
      const description = generatePixelRestorationDescription([dev]);

      expect(description.startsWith('Lab: mock\\-lab\\-mtv')).toBe(true);
    });

    it('generates prepopulated device data for a single device', () => {
      const dev = createMockDevice({ id: 'pixel-single' });
      const description = generatePixelRestorationDescription([dev]);

      expect(description.startsWith('Lab: mock\\-lab\\-mtv')).toBe(true);
      expect(description).toContain(
        '### Selected Devices for Restoration (1 device)',
      );
      expect(description).toContain('[pixel\\-single]');
      expect(description).toContain('**Run Target:** oriole');
    });

    it('generates table, breakdown, and Fleet Console link for multiple devices', () => {
      const dev1 = createMockDevice({ id: 'pixel-1' });
      const dev2 = createMockDevice({ id: 'pixel-2' });
      const description = generatePixelRestorationDescription([dev1, dev2]);

      expect(description.startsWith('Lab: mock\\-lab\\-mtv')).toBe(true);
      expect(description).toContain(
        '### Selected Devices for Restoration (2 devices)',
      );
      expect(description).toContain(
        '| Device ID | Hostname | Run Target | State | Mobile Harness |',
      );
      expect(description).toContain('[pixel\\-1]');
      expect(description).toContain('[pixel\\-2]');
      expect(description).toContain('[View in MH](');
      expect(description).toContain('### Device Details');
      expect(description).toContain(
        '[View selected devices in Fleet Console](https://ci.chromium.org/ui/fleet/p/pixel/devices?filters=',
      );

      const headerPos = description.indexOf(
        '### Selected Devices for Restoration (2 devices)',
      );
      const fconLinkPos = description.indexOf(
        '[View selected devices in Fleet Console]',
      );
      const tablePos = description.indexOf('| Device ID | Hostname |');
      const detailsPos = description.indexOf('### Device Details');

      expect(headerPos).toBeGreaterThan(-1);
      expect(fconLinkPos).toBeGreaterThan(headerPos);
      expect(tablePos).toBeGreaterThan(fconLinkPos);
      expect(detailsPos).toBeGreaterThan(tablePos);
    });

    it('adapts gracefully for a large batch of devices without URL overflow', () => {
      const largeBatch = Array.from({ length: 80 }, (_, i) =>
        createMockDevice({
          id: `pixel-batch-device-${i}`,
          omnilabSpec: {
            labels: {
              lab_name: { values: ['mock-lab-mtv'] },
              hostname: { values: [`host-${i}.mock-lab-mtv`] },
            },
          },
        }),
      );

      const description = generatePixelRestorationDescription(largeBatch);
      expect(description.startsWith('Lab: mock\\-lab\\-mtv')).toBe(true);
      expect(description.length).toBeLessThan(4000);
      expect(description).toContain(
        '### Selected Devices for Restoration (80 devices)',
      );
      // Minimal mode: device IDs are listed inline and escaped.
      expect(description).toContain(
        '**Device IDs:** pixel\\-batch\\-device\\-0, pixel\\-batch\\-device\\-1, ',
      );
      expect(description).not.toContain('pixel-batch-device-0,');
    });

    it('escapes markdown control characters in lab name and device IDs', () => {
      const dev1 = createMockDevice({
        id: 'dev_[1](x)',
        omnilabSpec: {
          labels: {
            lab_name: { values: ['lab*[evil](http://x)'] },
            hostname: { values: ['host-1'] },
          },
        },
      });
      const dev2 = createMockDevice({
        id: 'dev_2',
        omnilabSpec: {
          labels: {
            lab_name: { values: ['lab*[evil](http://x)'] },
            hostname: { values: ['host-2'] },
          },
        },
      });

      const description = generatePixelRestorationDescription([dev1, dev2]);
      expect(
        description.startsWith('Lab: lab\\*\\[evil\\]\\(http://x\\)'),
      ).toBe(true);
      expect(description).not.toContain('Lab: lab*[evil](http://x)');
      // The Fleet Console link URL must not be escaped.
      expect(description).toContain(
        '[View selected devices in Fleet Console](https://ci.chromium.org/ui/fleet/p/pixel/devices?filters=' +
          encodeURIComponent('id = "dev_[1](x)" OR id = "dev_2"') +
          ')',
      );
    });

    it('retains View selected devices in Fleet Console link at top when filter string exceeds length limit', () => {
      const batch = Array.from({ length: 30 }, (_, i) =>
        createMockDevice({
          id: `pixel-batch-long-device-identifier-${i}`,
          omnilabSpec: {
            labels: {
              lab_name: { values: ['mock-lab-mtv'] },
              hostname: { values: [`host-${i}.mock-lab-mtv`] },
            },
          },
        }),
      );

      const description = generatePixelRestorationDescription(batch);
      expect(description).toContain(
        '[View selected devices in Fleet Console](https://ci.chromium.org/ui/fleet/p/pixel/devices) (Device filter omitted due to length)',
      );
      const headerPos = description.indexOf(
        '### Selected Devices for Restoration (30 devices)',
      );
      const fconLinkPos = description.indexOf(
        '[View selected devices in Fleet Console]',
      );
      expect(headerPos).toBeGreaterThan(-1);
      expect(fconLinkPos).toBeGreaterThan(headerPos);
    });

    it('preserves Mobile Harness links in compact mode table', () => {
      // 10 devices to trigger compact mode (full mode ~3512 chars > 3500, compact mode ~2762 chars <= 3500)
      const batch = Array.from({ length: 10 }, (_, i) =>
        createMockDevice({
          id: `pixel-compact-device-${i}`,
          omnilabSpec: {
            labels: {
              lab_name: { values: ['mock-lab-mtv'] },
              hostname: { values: [`host-${i}.mock-lab-mtv`] },
              host_ip: { values: ['10.0.0.1'] },
            },
          },
        }),
      );

      const description = generatePixelRestorationDescription(batch);
      expect(description).toContain('| Device ID | Hostname |');
      expect(description).toContain('[View in MH](');
      expect(description).toContain(getMobileHarnessLink(batch[0]));
    });
  });

  describe('PixelRestorationConfig', () => {
    it('has correct componentId and templateId', () => {
      expect(PixelRestorationConfig.componentId).toBe(
        PIXEL_RESTORATION_COMPONENT_ID,
      );
      expect(PixelRestorationConfig.componentId).toBe('2174751');
      expect(PixelRestorationConfig.getTemplateId([createMockDevice()])).toBe(
        PIXEL_RESTORATION_TEMPLATE_ID,
      );
      expect(PixelRestorationConfig.getTemplateId([createMockDevice()])).toBe(
        '2403455',
      );
    });

    it('uses https://b.corp.google.com as baseUrl', () => {
      expect(PixelRestorationConfig.baseUrl).toBe('https://b.corp.google.com');
    });
  });
});
