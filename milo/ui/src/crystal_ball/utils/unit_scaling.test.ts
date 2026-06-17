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
  detectRawUnit,
  findUnitInfo,
  getScaleFactor,
  scaleValue,
} from './unit_scaling';

describe('Unit Scaling Utilities', () => {
  describe('findUnitInfo', () => {
    it('should find units by ID or aliases case-insensitively', () => {
      expect(findUnitInfo('B')?.id).toBe('B');
      expect(findUnitInfo('bytes')?.id).toBe('B');
      expect(findUnitInfo('MB')?.id).toBe('MB');
      expect(findUnitInfo('megabytes')?.id).toBe('MB');
      expect(findUnitInfo('ns')?.id).toBe('ns');
      expect(findUnitInfo('nanoseconds')?.id).toBe('ns');
      expect(findUnitInfo('ms')?.id).toBe('ms');
      expect(findUnitInfo('milliseconds')?.id).toBe('ms');
      expect(findUnitInfo('')).toBeUndefined();
      expect(findUnitInfo('unknown_unit')).toBeUndefined();
    });
  });

  describe('detectRawUnit', () => {
    it('should detect storage units from metric name suffixes', () => {
      expect(detectRawUnit('dram_used_bytes')?.id).toBe('B');
      expect(detectRawUnit('memory_bytes_count')?.id).toBe('B');
      expect(detectRawUnit('file_size_kb')?.id).toBe('KB');
      expect(detectRawUnit('download_mb')?.id).toBe('MB');
      expect(detectRawUnit('disk_gb')?.id).toBe('GB');
    });

    it('should detect time units from metric name suffixes', () => {
      expect(detectRawUnit('latency_ns')?.id).toBe('ns');
      expect(detectRawUnit('duration_us')?.id).toBe('us');
      expect(detectRawUnit('startup_time_ms')?.id).toBe('ms');
      expect(detectRawUnit('render_time_sec')?.id).toBe('s');
      expect(detectRawUnit('uptime_s')?.id).toBe('s');
    });

    it('should return undefined if no unit suffix is found', () => {
      expect(detectRawUnit('cpu_usage_percentage')).toBeUndefined();
      expect(detectRawUnit('build_count')).toBeUndefined();
    });
  });

  describe('scaleValue', () => {
    it('should scale storage values correctly', () => {
      const rawB = findUnitInfo('B')!;
      const targetMB = findUnitInfo('MB')!;
      const targetGB = findUnitInfo('GB')!;

      expect(scaleValue(1048576, rawB, targetMB)).toBe(1);
      expect(scaleValue(1073741824, rawB, targetGB)).toBe(1);
    });

    it('should scale time values correctly', () => {
      const rawNs = findUnitInfo('ns')!;
      const rawMs = findUnitInfo('ms')!;
      const targetMs = findUnitInfo('ms')!;
      const targetS = findUnitInfo('s')!;

      expect(scaleValue(1000000, rawNs, targetMs)).toBe(1);
      expect(scaleValue(1000, rawMs, targetS)).toBe(1);
      expect(scaleValue(500, rawNs, targetS)).toBeCloseTo(0.0000005, 10);
    });

    it('should fall back to default raw units if raw unit is undefined', () => {
      const targetMB = findUnitInfo('MB')!; // storage category -> default raw B
      const targetMs = findUnitInfo('ms')!; // time category -> default raw ns

      expect(scaleValue(1048576, undefined, targetMB)).toBe(1); // 1048576 B = 1 MB
      expect(scaleValue(1000000, undefined, targetMs)).toBe(1); // 1000000 ns = 1 ms
    });

    it('should not scale if categories are incompatible', () => {
      const rawB = findUnitInfo('B')!;
      const targetMs = findUnitInfo('ms')!;

      expect(scaleValue(12345, rawB, targetMs)).toBe(12345);
    });
  });

  describe('getScaleFactor', () => {
    it('should compute storage scale factors correctly', () => {
      const rawB = findUnitInfo('B')!;
      const targetMB = findUnitInfo('MB')!;
      const targetGB = findUnitInfo('GB')!;

      expect(getScaleFactor(rawB, targetMB)).toBe(1 / (1024 * 1024));
      expect(getScaleFactor(rawB, targetGB)).toBe(1 / (1024 * 1024 * 1024));
    });

    it('should compute time scale factors correctly', () => {
      const rawNs = findUnitInfo('ns')!;
      const rawMs = findUnitInfo('ms')!;
      const targetMs = findUnitInfo('ms')!;
      const targetS = findUnitInfo('s')!;

      expect(getScaleFactor(rawNs, targetMs)).toBe(1e-6);
      expect(getScaleFactor(rawMs, targetS)).toBe(1e-3);
    });

    it('should fall back to default raw units if raw unit is undefined', () => {
      const targetMB = findUnitInfo('MB')!; // storage category -> default raw B
      const targetMs = findUnitInfo('ms')!; // time category -> default raw ns

      expect(getScaleFactor(undefined, targetMB)).toBe(1 / (1024 * 1024));
      expect(getScaleFactor(undefined, targetMs)).toBe(1e-6);
    });

    it('should return 1 if categories are incompatible', () => {
      const rawB = findUnitInfo('B')!;
      const targetMs = findUnitInfo('ms')!;

      expect(getScaleFactor(rawB, targetMs)).toBe(1);
    });
  });
});
