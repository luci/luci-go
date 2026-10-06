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

import { fuzzySubstring } from './fuzzy_sort';

const ACRONYMS: Record<string, string> = {
  id: 'ID',
  dut: 'DUT',
  os: 'OS',
  ip: 'IP',
  mac: 'MAC',
  ec: 'EC',
  ap: 'AP',
  ro: 'RO',
  rw: 'RW',
  sku: 'SKU',
  hwid: 'HWID',
  gsc: 'GSC',
  rpm: 'RPM',
  ufs: 'UFS',
  usb: 'USB',
  gpu: 'GPU',
  cpu: 'CPU',
  ram: 'RAM',
  ssd: 'SSD',
  wifi: 'Wi-Fi',
  bt: 'BT',
  imei: 'IMEI',
  sim: 'SIM',
  uuid: 'UUID',
  url: 'URL',
  vm: 'VM',
  abi: 'ABI',
  sdk: 'SDK',
  adb: 'ADB',
};

const EXACT_HEADER_OVERRIDES: Record<string, string> = {
  id: 'ID',
  dut_id: 'DUT ID',
  'Dut ID': 'DUT ID',
  dut_state: 'DUT State',
  'Dut Name': 'DUT Name',
  'Lease state': 'Lease State',
  'Offline since': 'Offline Since',
  dut_name: 'DUT Name',
  current_task: 'Current Task',
  ufs_zone: 'UFS Zone',
  location_tag: 'location_tag',
  hardware: 'hardware',
  servo_hostname: 'Servo Hostname',
  servo_port: 'Servo Port',
  servo_serial: 'Servo Serial',
  servo_type: 'Servo Type',
  sw_version: 'SW Version',
};

/**
 * Formats a raw dimension or column key (e.g. `label-servo_state`, `dut_state`,
 * `sw_version`) into a human-readable header label (`Servo State`, `DUT State`,
 * `SW Version`) while preserving acronyms.
 */
export function humanizeColumnLabel(raw: string): string {
  if (!raw) return raw;
  if (EXACT_HEADER_OVERRIDES[raw]) {
    return EXACT_HEADER_OVERRIDES[raw];
  }

  if (/[A-Z]/.test(raw) && !raw.includes('_') && !raw.startsWith('label-')) {
    return raw;
  }

  const stripped = raw.replace(/^label-/, '');
  if (EXACT_HEADER_OVERRIDES[stripped]) {
    return EXACT_HEADER_OVERRIDES[stripped];
  }

  return stripped
    .split(/[_-]+/)
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase();
      if (ACRONYMS[lower]) {
        return ACRONYMS[lower];
      }
      return lower.charAt(0).toUpperCase() + lower.slice(1);
    })
    .join(' ');
}

/**
 * Returns backwards-compatible search terms for a column or filter option so
 * users can search by either the humanized display label (`Servo State`), the
 * historic raw key (`label-servo_state`), or the underscore/space variants
 * (`servo_state`, `servo state`).
 */
export function getColumnSearchAliases(id: string, label: string): string[] {
  const aliases = new Set<string>();
  if (label) aliases.add(label);
  if (id) {
    const stripped = id.replace(/^label-/, '');
    aliases.add(id);
    aliases.add(stripped);
    aliases.add(stripped.replace(/[_-]+/g, ' '));
    aliases.add(id.replace(/[_-]+/g, ' '));
    aliases.add(humanizeColumnLabel(id));
  }
  return Array.from(aliases);
}

export interface ColumnSearchMatch {
  score: number;
  matches: number[];
  matchedKey?: string;
  keyMatches?: number[];
}

/**
 * Evaluates a search query against both the humanized display label and the
 * underlying backend key. Returns `matchedKey` + `keyMatches` only when the
 * search matched via the backend key rather than the display label, keeping the
 * UI clean during normal searches while explaining backend-key matches.
 */
export function resolveColumnSearchMatch(
  searchQuery: string,
  rawKey: string,
  label: string,
): ColumnSearchMatch {
  const trimmed = searchQuery.trim();
  if (!trimmed) {
    return { score: 0, matches: [] };
  }

  const labelMatch = fuzzySubstring(trimmed, label);
  const rawKeyMatch = fuzzySubstring(trimmed, rawKey);
  const spaceNormalizedKey = rawKey.replace(/[_-]/g, ' ');
  const spaceKeyMatch = fuzzySubstring(trimmed, spaceNormalizedKey);

  // Iterating getColumnSearchAliases accounts for both stripped aliases (e.g. 'servo_state'
  // without 'label-') and humanized forms, granting an exact match score bonus (+10)
  // when the user types the shorter canonical key name.
  let bestAliasScore = Math.max(rawKeyMatch[0], spaceKeyMatch[0]);
  for (const alias of getColumnSearchAliases(rawKey, label)) {
    const [aliasScore] = fuzzySubstring(trimmed, alias);
    if (aliasScore > bestAliasScore) {
      bestAliasScore = aliasScore;
    }
  }

  const bestScore = Math.max(labelMatch[0], bestAliasScore);
  const cleanLabel = label.replace(/\s*\*$/, '');
  const isKeyDistinctFromLabel =
    Boolean(rawKey) && rawKey.toLowerCase() !== cleanLabel.toLowerCase();

  const showBackendKeyHint =
    isKeyDistinctFromLabel &&
    bestAliasScore > 0 &&
    (labelMatch[0] <= 0 || /[_-]/.test(trimmed));

  if (!showBackendKeyHint) {
    return {
      score: bestScore,
      matches: labelMatch[0] > 0 ? labelMatch[1] : [],
    };
  }

  let keyMatches: number[] = [];
  if (rawKeyMatch[0] > 0 && rawKeyMatch[0] >= spaceKeyMatch[0]) {
    keyMatches = rawKeyMatch[1];
  } else if (spaceKeyMatch[0] > 0) {
    keyMatches = spaceKeyMatch[1];
  } else {
    // When rawKey has a prefix like 'label-' and only the stripped alias matched,
    // compute matches on the stripped key offset by prefix length so match indices
    // map accurately to rawKey.
    const strippedPrefixMatch = /^label-/.exec(rawKey);
    const prefixLen = strippedPrefixMatch ? strippedPrefixMatch[0].length : 0;
    const strippedKey = rawKey.slice(prefixLen);
    const strippedMatch = fuzzySubstring(trimmed, strippedKey);
    const strippedSpaceMatch = fuzzySubstring(
      trimmed,
      strippedKey.replace(/[_-]/g, ' '),
    );
    if (strippedMatch[0] > 0) {
      keyMatches = strippedMatch[1].map((idx) => idx + prefixLen);
    } else if (strippedSpaceMatch[0] > 0) {
      keyMatches = strippedSpaceMatch[1].map((idx) => idx + prefixLen);
    }
  }

  return {
    score: bestScore,
    matches: labelMatch[0] > 0 ? labelMatch[1] : [],
    matchedKey: rawKey,
    keyMatches: keyMatches.length > 0 ? keyMatches : [],
  };
}
