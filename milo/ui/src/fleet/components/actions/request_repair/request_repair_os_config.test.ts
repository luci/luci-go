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

import { DutToRepair } from '../shared/types';

import { ChromeOSRepairConfig } from './request_repair_os_config';

describe('ChromeOSRepairConfig', () => {
  describe('generateTitle', () => {
    it('should generate correct title for ChromeOS repair', () => {
      const duts: DutToRepair[] = [
        {
          name: 'dut1',
          board: 'board1',
          model: 'model1',
          pool: 'pool1',
          dutId: 'dut1',
        },
      ];
      const title = ChromeOSRepairConfig.generateTitle(duts);
      expect(title).toBe(
        '[Location Unknown][Repair][board1.model1] Pool: [pool1] [dut1]',
      );
    });
  });

  describe('generateDescription', () => {
    it('should generate correct description for ChromeOS repair', () => {
      const duts: DutToRepair[] = [
        {
          name: 'dut1',
          board: 'board1',
          model: 'model1',
          pool: 'pool1',
          dutId: 'dut1',
        },
      ];
      const desc = ChromeOSRepairConfig.generateDescription(duts);
      expect(desc).toContain('**DUT Link(s) / Locations:**:');
      expect(desc).toContain(
        '* [http://goto.google.com/fcdut/dut1](http://goto.google.com/fcdut/dut1) (Location: \\<Please add if known\\>, Board: board1, Model: model1, Pool: pool1)',
      );
    });

    it('should sanitize markdown special characters in DUT fields', () => {
      const duts: DutToRepair[] = [
        {
          name: 'dut_1',
          board: 'board*bold*',
          model: 'model [link](http://evil.com)',
          pool: 'pool | col',
          dutId: 'dut_1',
        },
      ];
      const desc = ChromeOSRepairConfig.generateDescription(duts);
      expect(desc).toContain(
        '* [http://goto.google.com/fcdut/dut\\_1](http://goto.google.com/fcdut/dut_1) (Location: \\<Please add if known\\>, Board: board\\*bold\\*, Model: model \\[link\\]\\(http://evil\\.com\\), Pool: pool \\| col)',
      );
    });
  });

  describe('customFields', () => {
    it('should generate ChromeOS repair custom fields for a single DUT in needs_manual_repair', () => {
      const duts: DutToRepair[] = [
        {
          name: 'dut1',
          board: 'demo-board-alpha',
          model: 'demo-model-one',
          pool: 'pool1, pool2',
          dutId: 'C123456',
          state: 'needs_manual_repair',
          servoState: 'WORKING',
          servoUsbState: 'NORMAL',
          servoSerial: 'SERVOV4P1-S-2105',
        },
      ];
      const getFields = ChromeOSRepairConfig.customFields as (
        items: DutToRepair[],
      ) => string[];
      expect(getFields(duts)).toEqual([
        '998417:dut1',
        '1180197:C123456',
        '994275:demo-board-alpha',
        '994317:demo-model-one',
        '1061740:pool1, pool2',
        '1002699:needs_manual_repair',
        '1002733:WORKING',
        '1002657:NORMAL',
        '1180248:SERVOV4P1-S-2105',
        '994318:1',
      ]);
    });

    it('should omit Host Name when a single DUT is not in a manual-repair state', () => {
      const getFields = ChromeOSRepairConfig.customFields as (
        items: DutToRepair[],
      ) => string[];
      expect(
        getFields([
          {
            name: 'dut1',
            board: 'demo-board-alpha',
            model: 'demo-model-one',
            pool: 'pool1',
            dutId: 'C123456',
            state: 'ready',
          },
        ]),
      ).toEqual([
        '1180197:C123456',
        '994275:demo-board-alpha',
        '994317:demo-model-one',
        '1061740:pool1',
        '1002699:ready',
        '994318:1',
      ]);
    });

    it('should omit DUT Asset # when dutId is empty or default 0', () => {
      const getFields = ChromeOSRepairConfig.customFields as (
        items: DutToRepair[],
      ) => string[];
      expect(
        getFields([
          {
            name: 'dut1',
            board: 'demo-board-alpha',
            model: 'demo-model-one',
            pool: 'pool1',
            dutId: '',
            state: 'needs_replacement',
          },
        ]),
      ).toEqual([
        '998417:dut1',
        '994275:demo-board-alpha',
        '994317:demo-model-one',
        '1061740:pool1',
        '1002699:needs_replacement',
        '994318:1',
      ]);
      expect(
        getFields([
          {
            name: 'dut1',
            board: 'demo-board-alpha',
            model: 'demo-model-one',
            pool: 'pool1',
            dutId: '0',
            state: 'NEEDS_MANUAL_REPAIR',
          },
        ]),
      ).toEqual([
        '998417:dut1',
        '994275:demo-board-alpha',
        '994317:demo-model-one',
        '1061740:pool1',
        '1002699:NEEDS_MANUAL_REPAIR',
        '994318:1',
      ]);
    });

    it('should omit Host Name and aggregate unique custom field values when multiple DUTs are selected', () => {
      const getFields = ChromeOSRepairConfig.customFields as (
        items: DutToRepair[],
      ) => string[];
      expect(
        getFields([
          {
            name: 'dut1',
            board: 'demo-board-alpha',
            model: 'demo-model-one',
            pool: 'pool1, pool2',
            dutId: 'C123456',
            state: 'needs_manual_repair',
            servoState: 'NOT_CONNECTED',
            servoUsbState: 'HARDWARE_NOT_DETECTED',
          },
          {
            name: 'dut2',
            board: 'demo-board-alpha',
            model: 'demo-model-two',
            pool: 'pool2, pool3',
            dutId: 'C654321',
            state: 'needs_manual_repair',
            servoState: 'NOT_CONNECTED',
            servoUsbState: 'NORMAL',
          },
        ]),
      ).toEqual([
        '1180197:C123456, C654321',
        '994275:demo-board-alpha',
        '994317:demo-model-one, demo-model-two',
        '1061740:pool1, pool2, pool3',
        '1002699:needs_manual_repair',
        '1002733:NOT_CONNECTED',
        '1002657:HARDWARE_NOT_DETECTED, NORMAL',
        '994318:2',
      ]);
    });
  });
});
