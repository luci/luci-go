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
        '* http://go/fcdut/dut1 (Location: <Please add if known>, Board: board1, Model: model1, Pool: pool1)',
      );
    });
  });
});
