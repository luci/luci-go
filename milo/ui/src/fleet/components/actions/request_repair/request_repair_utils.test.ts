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

import { extractHostname, extractPool } from './request_repair_utils';

describe('request_repair_utils', () => {
  describe('extractHostname', () => {
    it('should extract hostname from ufsLabels if available', () => {
      const device = {
        id: 'machine1',
        ufsLabels: {
          hostname: { values: ['host1'] },
        },
      };
      expect(extractHostname(device)).toBe('host1');
    });

    it('should fallback to hostname property if ufsLabels not available', () => {
      const device = {
        id: 'machine1',
        hostname: 'host1',
      };
      expect(extractHostname(device)).toBe('host1');
    });

    it('should fallback to id if neither is available', () => {
      const device = {
        id: 'machine1',
      };
      expect(extractHostname(device)).toBe('machine1');
    });
  });

  describe('extractPool', () => {
    it('should extract pool from swarmingLabels if available', () => {
      const device = {
        swarmingLabels: {
          pool: { values: ['pool1'] },
        },
      };
      expect(extractPool(device)).toBe('pool1');
    });

    it('should return undefined if not available', () => {
      const device = {};
      expect(extractPool(device)).toBeUndefined();
    });
  });
});
