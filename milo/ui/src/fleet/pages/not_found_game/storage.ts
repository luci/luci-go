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

import { GhostFrame, StageRecord, StarRating } from './types';

const STORAGE_KEY = 'fleet_console_10s_bass_records_v2';

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isStarRating(value: unknown): value is StarRating {
  return value === 0 || value === 1 || value === 2 || value === 3;
}

function isGhostFrame(value: unknown): value is GhostFrame {
  if (!isObjectRecord(value)) return false;
  return (
    typeof value.t === 'number' &&
    typeof value.x === 'number' &&
    typeof value.y === 'number' &&
    (value.facing === 1 || value.facing === -1) &&
    typeof value.slashing === 'boolean'
  );
}

function parseRecord(value: unknown): StageRecord | null {
  if (!isObjectRecord(value)) return null;
  if (typeof value.bestTimeMs !== 'number' || !isStarRating(value.stars)) {
    return null;
  }
  const ghost = Array.isArray(value.ghost)
    ? value.ghost.filter(isGhostFrame)
    : [];
  const splitTimesMs = Array.isArray(value.splitTimesMs)
    ? value.splitTimesMs.filter((n): n is number => typeof n === 'number')
    : [];
  const devMedal = Boolean(value.devMedal);
  const tasMedal = Boolean(value.tasMedal);
  return {
    bestTimeMs: value.bestTimeMs,
    stars: value.stars,
    devMedal,
    tasMedal,
    splitTimesMs,
    ghost,
  };
}

let memoryRecords: Record<string, StageRecord> = {};

export function loadStageRecords(): Record<string, StageRecord> {
  if (typeof window === 'undefined') return { ...memoryRecords };
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...memoryRecords };
    const parsed: unknown = JSON.parse(raw);
    if (!isObjectRecord(parsed)) return { ...memoryRecords };
    const result: Record<string, StageRecord> = {};
    for (const [key, val] of Object.entries(parsed)) {
      const rec = parseRecord(val);
      if (rec) {
        result[key] = rec;
      }
    }
    memoryRecords = result;
    return result;
  } catch {
    return { ...memoryRecords };
  }
}

export function saveStageRecord(
  stageId: string,
  record: StageRecord,
): Record<string, StageRecord> {
  const records = loadStageRecords();
  const next: Record<string, StageRecord> = {
    ...records,
    [stageId]: record,
  };
  memoryRecords = next;
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      // Ignore storage quota / sandboxed null-origin SecurityError.
    }
  }
  return next;
}
