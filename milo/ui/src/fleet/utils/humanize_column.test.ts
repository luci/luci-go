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
  getColumnSearchAliases,
  humanizeColumnLabel,
  resolveColumnSearchMatch,
} from './humanize_column';

describe('humanizeColumnLabel', () => {
  it('humanizes raw label-* and snake_case keys while preserving acronyms', () => {
    expect(humanizeColumnLabel('id')).toBe('ID');
    expect(humanizeColumnLabel('dut_id')).toBe('DUT ID');
    expect(humanizeColumnLabel('dut_state')).toBe('DUT State');
    expect(humanizeColumnLabel('label-servo_state')).toBe('Servo State');
    expect(humanizeColumnLabel('label-board')).toBe('Board');
    expect(humanizeColumnLabel('label-os_type')).toBe('OS Type');
    expect(humanizeColumnLabel('sw_version')).toBe('SW Version');
  });

  it('preserves already humanized labels', () => {
    expect(humanizeColumnLabel(' Associated Hostname')).toBe(
      ' Associated Hostname',
    );
  });
});

describe('getColumnSearchAliases', () => {
  it('includes both historic raw keys and humanized names', () => {
    const aliases = getColumnSearchAliases('label-servo_state', 'Servo State');
    expect(aliases).toContain('Servo State');
    expect(aliases).toContain('label-servo_state');
    expect(aliases).toContain('servo_state');
    expect(aliases).toContain('servo state');
  });
});

describe('resolveColumnSearchMatch', () => {
  it('hides backend key hint when query directly matches humanized label', () => {
    const res = resolveColumnSearchMatch(
      'servo',
      'label-servo_state',
      'Servo State',
    );
    expect(res.score).toBeGreaterThan(0);
    expect(res.matches).toEqual([0, 1, 2, 3, 4]);
    expect(res.matchedKey).toBeUndefined();
  });

  it('reveals highlighted backend key hint when query matches raw backend key', () => {
    const res = resolveColumnSearchMatch(
      'label-servo_state',
      'label-servo_state',
      'Servo State',
    );
    expect(res.score).toBeGreaterThan(0);
    expect(res.matchedKey).toBe('label-servo_state');
    expect(res.keyMatches).toHaveLength('label-servo_state'.length);
  });

  it('returns zero score on empty or whitespace-only search query', () => {
    const res = resolveColumnSearchMatch(
      '   ',
      'label-servo_state',
      'Servo State',
    );
    expect(res.score).toBe(0);
    expect(res.matches).toEqual([]);
    expect(res.matchedKey).toBeUndefined();
  });

  it('maps keyMatches accurately when query matches stripped alias without prefix', () => {
    const res = resolveColumnSearchMatch(
      'servo_state',
      'label-servo_state',
      'Servo State',
    );
    expect(res.score).toBeGreaterThan(0);
    expect(res.matchedKey).toBe('label-servo_state');
    // 'label-' is 6 chars, so 'servo_state' matches indices 6 through 16
    expect(res.keyMatches).toEqual([6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]);
  });
});
