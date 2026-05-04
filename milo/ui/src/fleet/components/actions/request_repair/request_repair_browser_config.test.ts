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
  BrowserDeviceToRepair,
  BrowserRepairConfig,
} from './request_repair_browser_config';

describe('BrowserRepairConfig', () => {
  describe('generateTitle', () => {
    it('should generate correct title for single device', () => {
      const devices = [{ id: 'machine1', hostname: 'host1' }];
      const title = BrowserRepairConfig.generateTitle(devices);
      expect(title).toBe('[Unknown Zone][Browser][Repair][host1]');
    });

    it('should generate correct title for exactly 2 devices', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1' },
        { id: 'machine2', hostname: 'host2' },
      ];
      const title = BrowserRepairConfig.generateTitle(devices);
      expect(title).toBe('[Unknown Zone][Browser][Repair][host1, host2]');
    });

    it('should generate correct title with zone when available', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', zone: 'SFO36_BROWSER' },
      ];
      const title = BrowserRepairConfig.generateTitle(devices);
      expect(title).toBe('[SFO36_BROWSER][Browser][Repair][host1]');
    });

    it('should generate correct title with Multiple Zones when available', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', zone: 'SFO36_BROWSER' },
        { id: 'machine2', hostname: 'host2', zone: 'IAD65_BROWSER' },
      ];
      const title = BrowserRepairConfig.generateTitle(devices);
      expect(title).toBe('[2 zones][Browser][Repair][host1, host2]');
    });

    it('should generate correct title for bulk devices', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1' },
        { id: 'machine2', hostname: 'host2' },
        { id: 'machine3', hostname: 'host3' },
      ];
      const title = BrowserRepairConfig.generateTitle(devices);
      expect(title).toBe(
        '[Unknown Zone][Browser] [Repair] [3] - [Multiple Devices]',
      );
    });
  });

  describe('generateDescription', () => {
    it('should generate correct description for single repair', () => {
      const devices = [{ id: 'machine1', hostname: 'host1' }];
      const desc = BrowserRepairConfig.generateDescription(devices);
      expect(desc).toContain('* **Hostname:** host1');
      expect(desc).toContain(
        '* **Machine:** [machine1](https://ci.chromium.org/ui/fleet/p/chromium/devices/machine1)',
      );
    });

    it('should generate correct description for bulk repair', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', pool: 'pool1' },
        { id: 'machine2', hostname: 'host2', pool: 'pool2' },
      ];
      const desc = BrowserRepairConfig.generateDescription(devices);
      const poolUrl = `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."pool" = "pool1"`)}`;
      expect(desc).toContain('* **Hostname:** host1');
      expect(desc).toContain(
        `    * **Machine:** [machine1](https://ci.chromium.org/ui/fleet/p/chromium/devices/machine1)`,
      );
      expect(desc).toContain(`    * **Pool:** [pool1](${poolUrl})`);
    });
  });

  describe('hotlistIds', () => {
    it('should return default hotlist when no zone matches', () => {
      const devices = [{ id: 'machine1', hostname: 'host1' }];
      const hotlists = (
        BrowserRepairConfig.hotlistIds as (
          items: BrowserDeviceToRepair[],
        ) => string
      )(devices);
      expect(hotlists).toBe('7555487');
    });

    it('should return multiple hotlists when zones match', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', zone: 'SFO36_BROWSER' },
        { id: 'machine2', hostname: 'host2', zone: 'IAD65_BROWSER' },
      ];
      const hotlists = (
        BrowserRepairConfig.hotlistIds as (
          items: BrowserDeviceToRepair[],
        ) => string
      )(devices);
      const expected = ['7555487', '6458008', '6909561'].sort().join(',');
      expect(hotlists.split(',').sort().join(',')).toBe(expected);
    });

    it('should handle lowercase zones', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', zone: 'sfo36_browser' },
      ];
      const hotlists = (
        BrowserRepairConfig.hotlistIds as (
          items: BrowserDeviceToRepair[],
        ) => string
      )(devices);
      expect(hotlists.split(',')).toContain('6458008');
    });

    it('should put zone-specific hotlists first', () => {
      const devices = [
        { id: 'machine1', hostname: 'host1', zone: 'SFO36_BROWSER' },
      ];
      const hotlists = (
        BrowserRepairConfig.hotlistIds as (
          items: BrowserDeviceToRepair[],
        ) => string
      )(devices);
      expect(hotlists).toBe('6458008,7555487');
    });
  });
});
