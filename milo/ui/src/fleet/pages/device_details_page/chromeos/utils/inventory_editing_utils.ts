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

import { DeviceConfigEdits } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';
import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

export const isLabstationConfig = (
  lse: MachineLSE | null | undefined,
): boolean => {
  return Boolean(lse?.chromeosMachineLse?.deviceLse?.labstation);
};

export const LOGICAL_SCHEDULING_PATHS = {
  dutPools: 'chromeosMachineLse.deviceLse.dut.pools',
  labstationPools: 'chromeosMachineLse.deviceLse.labstation.pools',
};

export interface FieldConfig {
  label: string;
  path: string;
  editPath: string;
  type: 'string' | 'number' | 'array';
}

export const getEditableFields = (isLabstation: boolean): FieldConfig[] => [
  {
    label: 'Pools',
    path: isLabstation
      ? LOGICAL_SCHEDULING_PATHS.labstationPools
      : LOGICAL_SCHEDULING_PATHS.dutPools,
    editPath: 'pools',
    type: 'array',
  },
];

export const getSegments = (path: string | string[]): string[] => {
  return typeof path === 'string' ? path.split('.') : path;
};

export const getNestedValue = (
  obj: unknown,
  path: string | string[],
): unknown => {
  if (!obj || typeof obj !== 'object') return undefined;
  const segments = getSegments(path);
  let curr = obj as Record<string, unknown>;
  for (const key of segments) {
    if (curr === null || curr === undefined || typeof curr !== 'object') {
      return undefined;
    }
    curr = curr[key] as Record<string, unknown>;
  }
  return curr;
};

export const mutateNestedValue = (
  obj: Record<string, unknown>,
  path: string | string[],
  value: unknown,
): void => {
  const segments = getSegments(path);
  let curr = obj;
  for (let i = 0; i < segments.length - 1; i++) {
    const key = segments[i];
    if (!(key in curr) || typeof curr[key] !== 'object' || curr[key] === null) {
      curr[key] = {};
    }
    curr = curr[key] as Record<string, unknown>;
  }
  const lastKey = segments[segments.length - 1];
  curr[lastKey] = value;
};

export const updateNestedValues = (
  obj: Record<string, unknown>,
  updates: Array<{ path: string | string[]; value: unknown }>,
): Record<string, unknown> => {
  // Deep copy using JSON serialization. This is safe for plain proto-based objects
  // (without functions/Dates) and avoids structuredClone compatibility issues
  // during Jest execution in Node test environments.
  const copy = JSON.parse(JSON.stringify(obj));
  updates.forEach(({ path, value }) => {
    mutateNestedValue(copy, path, value);
  });
  return copy;
};

export const areArraysEqual = (a: string[], b: string[]): boolean => {
  if (a.length !== b.length) return false;
  const aSorted = [...a].sort();
  const bSorted = [...b].sort();
  return aSorted.every((val, idx) => val === bSorted[idx]);
};

export interface FieldDiff {
  path: string;
  original: string;
  updated: string;
}

export const calculateDiff = (
  original: MachineLSE | null | undefined,
  updated: MachineLSE | null | undefined,
): FieldDiff[] => {
  const diffs: FieldDiff[] = [];
  if (!original || !updated) return diffs;

  const isLabstation = isLabstationConfig(original);
  const fields = getEditableFields(isLabstation);

  fields.forEach(({ label, path, type }) => {
    const origVal = getNestedValue(original, path);
    const draftVal = getNestedValue(updated, path);

    if (type === 'array') {
      const origArr = Array.isArray(origVal) ? (origVal as string[]) : [];
      const draftArr = Array.isArray(draftVal) ? (draftVal as string[]) : [];
      if (!areArraysEqual(origArr, draftArr)) {
        diffs.push({
          path: label,
          original: origArr.join(',') || '(empty)',
          updated: draftArr.join(',') || '(empty)',
        });
      }
    } else {
      const origStr =
        origVal !== undefined && origVal !== null ? String(origVal) : '';
      const draftStr =
        draftVal !== undefined && draftVal !== null ? String(draftVal) : '';
      if (origStr !== draftStr) {
        diffs.push({
          path: label,
          original: origStr || '(empty)',
          updated: draftStr || '(empty)',
        });
      }
    }
  });

  return diffs;
};

export const translateDiffToEdits = (
  original: MachineLSE | null | undefined,
  updated: MachineLSE | null | undefined,
): { edits: Partial<DeviceConfigEdits>; paths: string[] } => {
  const edits: Record<string, unknown> = {};
  const paths: string[] = [];
  if (!original || !updated) return { edits: {}, paths };

  const isLabstation = isLabstationConfig(original);
  const fields = getEditableFields(isLabstation);

  fields.forEach(({ path, editPath, type }) => {
    const origVal = getNestedValue(original, path);
    const draftVal = getNestedValue(updated, path);

    let hasChanged = false;
    if (type === 'array') {
      const origArr = Array.isArray(origVal) ? (origVal as string[]) : [];
      const draftArr = Array.isArray(draftVal) ? (draftVal as string[]) : [];
      hasChanged = !areArraysEqual(origArr, draftArr);
    } else {
      hasChanged = origVal !== draftVal;
    }

    if (hasChanged) {
      edits[editPath] = type === 'array' ? (draftVal ?? []) : draftVal;
      paths.push(editPath);
    }
  });

  return { edits: edits as Partial<DeviceConfigEdits>, paths };
};
