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

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import {
  areArraysEqual,
  calculateDiff,
  getNestedValue,
  mutateNestedValue,
  translateDiffToEdits,
  updateNestedValues,
} from './inventory_editing_utils';

describe('inventory_editing_utils', () => {
  describe('getNestedValue', () => {
    it('gets a nested property by string path', () => {
      const obj = {
        a: {
          b: {
            c: 'val',
          },
        },
      };
      expect(getNestedValue(obj, 'a.b.c')).toBe('val');
    });

    it('returns undefined if path is missing', () => {
      const obj = { a: {} };
      expect(getNestedValue(obj, 'a.b.c')).toBeUndefined();
    });
  });

  describe('mutateNestedValue', () => {
    it('mutates a nested property by path', () => {
      const obj = {
        a: {
          b: {
            c: 'old',
          },
        },
      };
      mutateNestedValue(obj, 'a.b.c', 'new');
      expect(obj.a.b.c).toBe('new');
    });

    it('creates intermediate objects if path segments are missing', () => {
      const obj: Record<string, unknown> = {};
      mutateNestedValue(obj, 'a.b.c', 'new');
      const a = obj.a as Record<string, unknown>;
      const b = a.b as Record<string, unknown>;
      expect(b.c).toBe('new');
    });
  });

  describe('updateNestedValues', () => {
    it('returns a cloned object with multiple nested changes', () => {
      const obj = {
        a: { b: '1' },
        x: { y: '2' },
      };
      interface ExpectedResult {
        a: { b: string };
        x: { y: string };
      }
      const result = updateNestedValues(obj, [
        { path: 'a.b', value: '11' },
        { path: 'x.y', value: '22' },
      ]) as unknown as ExpectedResult;

      expect(result).not.toBe(obj);
      expect(result.a.b).toBe('11');
      expect(result.x.y).toBe('22');
      expect(obj.a.b).toBe('1'); // original is unchanged
    });
  });

  describe('areArraysEqual', () => {
    it('returns true if arrays are equal', () => {
      expect(areArraysEqual(['a', 'b'], ['a', 'b'])).toBe(true);
    });

    it('returns false if lengths differ', () => {
      expect(areArraysEqual(['a'], ['a', 'b'])).toBe(false);
    });

    it('returns true if values differ in order but are equivalent sets', () => {
      expect(areArraysEqual(['a', 'b'], ['b', 'a'])).toBe(true);
    });
  });

  describe('calculateDiff', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('returns empty diff if no changes', () => {
      const updated = JSON.parse(JSON.stringify(original));
      expect(calculateDiff(original, updated)).toEqual([]);
    });

    it('returns diff when array fields change', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.dut!.pools = ['pool1', 'pool3'];

      expect(calculateDiff(original, updated)).toEqual([
        {
          path: 'Pools',
          original: 'pool1,pool2',
          updated: 'pool1,pool3',
        },
      ]);
    });
  });

  describe('translateDiffToEdits', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('translates changed array fields to DeviceConfigEdits format', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.dut!.pools = ['pool1', 'pool3'];

      const result = translateDiffToEdits(original, updated);
      expect(result.edits).toEqual({
        pools: ['pool1', 'pool3'],
      });
      expect(result.paths).toEqual(['pools']);
    });
  });

  describe('calculateDiff (Labstation)', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          labstation: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('returns empty diff if no changes', () => {
      const updated = JSON.parse(JSON.stringify(original));
      expect(calculateDiff(original, updated)).toEqual([]);
    });

    it('returns diff when array fields change', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.labstation!.pools = [
        'pool1',
        'pool3',
      ];

      expect(calculateDiff(original, updated)).toEqual([
        {
          path: 'Pools',
          original: 'pool1,pool2',
          updated: 'pool1,pool3',
        },
      ]);
    });
  });

  describe('translateDiffToEdits (Labstation)', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          labstation: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('translates changed array fields to DeviceConfigEdits format', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.labstation!.pools = [
        'pool1',
        'pool3',
      ];

      const result = translateDiffToEdits(original, updated);
      expect(result.edits).toEqual({
        pools: ['pool1', 'pool3'],
      });
      expect(result.paths).toEqual(['pools']);
    });
  });
});
