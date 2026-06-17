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

export type UnitCategory = 'storage' | 'time';

export interface UnitInfo {
  readonly id: string;
  readonly label: string;
  readonly category: UnitCategory;
  readonly factor: number; // conversion factor relative to base unit (B for storage, s for time)
}

export const SUPPORTED_UNITS: readonly UnitInfo[] = [
  // Storage (using binary factors)
  { id: 'B', label: 'Bytes (B)', category: 'storage', factor: 1 },
  { id: 'KB', label: 'Kilobytes (KB)', category: 'storage', factor: 1024 },
  {
    id: 'MB',
    label: 'Megabytes (MB)',
    category: 'storage',
    factor: 1024 * 1024,
  },
  {
    id: 'GB',
    label: 'Gigabytes (GB)',
    category: 'storage',
    factor: 1024 * 1024 * 1024,
  },
  // Time (relative to seconds)
  { id: 'ns', label: 'Nanoseconds (ns)', category: 'time', factor: 1e-9 },
  { id: 'us', label: 'Microseconds (μs)', category: 'time', factor: 1e-6 },
  { id: 'ms', label: 'Milliseconds (ms)', category: 'time', factor: 1e-3 },
  { id: 's', label: 'Seconds (s)', category: 'time', factor: 1 },
];

const UNIT_ALIASES: Record<string, string> = {
  b: 'B',
  byte: 'B',
  bytes: 'B',
  gb: 'GB',
  gigabyte: 'GB',
  gigabytes: 'GB',
  kb: 'KB',
  kilobyte: 'KB',
  kilobytes: 'KB',
  mb: 'MB',
  megabyte: 'MB',
  megabytes: 'MB',
  micros: 'us',
  microsecond: 'us',
  microseconds: 'us',
  millis: 'ms',
  millisecond: 'ms',
  milliseconds: 'ms',
  ms: 'ms',
  nanosecond: 'ns',
  nanoseconds: 'ns',
  ns: 'ns',
  s: 's',
  sec: 's',
  second: 's',
  seconds: 's',
  us: 'us',
  μs: 'us',
};

/**
 * Finds the UnitInfo matching a display unit identifier or alias (case-insensitive).
 */
export function findUnitInfo(unitStr: string): UnitInfo | undefined {
  const normalized = unitStr.trim().toLowerCase();
  if (!normalized) return undefined;

  const targetId = UNIT_ALIASES[normalized] ?? normalized;
  return SUPPORTED_UNITS.find(
    (u) => u.id.toLowerCase() === targetId.toLowerCase(),
  );
}

/**
 * Detects the raw unit of a metric from its field name or key based on suffix conventions.
 */
export function detectRawUnit(metricName: string): UnitInfo | undefined {
  const normalized = metricName.toLowerCase();

  // Storage checks
  if (
    normalized.endsWith('_bytes') ||
    normalized.includes('_bytes_') ||
    normalized.endsWith('_b') ||
    normalized.includes('_b_')
  ) {
    return findUnitInfo('B');
  }
  if (
    normalized.endsWith('_kb') ||
    normalized.includes('_kb_') ||
    normalized.includes('kilobytes')
  ) {
    return findUnitInfo('KB');
  }
  if (
    normalized.endsWith('_mb') ||
    normalized.includes('_mb_') ||
    normalized.includes('megabytes')
  ) {
    return findUnitInfo('MB');
  }
  if (
    normalized.endsWith('_gb') ||
    normalized.includes('_gb_') ||
    normalized.includes('gigabytes')
  ) {
    return findUnitInfo('GB');
  }

  // Time checks
  if (
    normalized.endsWith('_ns') ||
    normalized.includes('_ns_') ||
    normalized.includes('nanoseconds')
  ) {
    return findUnitInfo('ns');
  }
  if (
    normalized.endsWith('_us') ||
    normalized.includes('_us_') ||
    normalized.includes('microseconds') ||
    normalized.includes('_micros')
  ) {
    return findUnitInfo('us');
  }
  if (
    normalized.endsWith('_ms') ||
    normalized.includes('_ms_') ||
    normalized.includes('milliseconds') ||
    normalized.includes('_millis')
  ) {
    return findUnitInfo('ms');
  }
  if (
    normalized.endsWith('_sec') ||
    normalized.includes('_sec_') ||
    normalized.endsWith('_s') ||
    normalized.includes('_s_') ||
    normalized.includes('seconds')
  ) {
    return findUnitInfo('s');
  }

  return undefined;
}

/**
 * Computes the scaling factor from a raw unit to the target display unit.
 */
export function getScaleFactor(
  rawUnit: UnitInfo | undefined,
  displayUnit: UnitInfo,
): number {
  if (!rawUnit) {
    // Default fallback: if no raw unit is detected, assume raw unit is the base unit of the target category.
    const defaultRawUnitId = displayUnit.category === 'storage' ? 'B' : 'ns';
    const defaultRawUnit = SUPPORTED_UNITS.find(
      (u) => u.id === defaultRawUnitId,
    );
    return getScaleFactor(defaultRawUnit, displayUnit);
  }

  if (rawUnit.category !== displayUnit.category) {
    // Incompatible categories; do not scale.
    return 1;
  }

  return rawUnit.factor / displayUnit.factor;
}

/**
 * Scales a raw value from its detected raw unit to the target display unit.
 */
export function scaleValue(
  value: number,
  rawUnit: UnitInfo | undefined,
  displayUnit: UnitInfo,
): number {
  return value * getScaleFactor(rawUnit, displayUnit);
}
