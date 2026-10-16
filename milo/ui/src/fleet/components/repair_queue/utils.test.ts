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

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';

import {
  extractRuleChips,
  formatPoints,
  getBugUrl,
  parsePriorityRules,
} from './utils';

describe('repair queue utils', () => {
  describe('formatPoints helper', () => {
    it('formats positive values with + and pts', () => {
      expect(formatPoints('350')).toBe('+350 pts');
      expect(formatPoints('+350')).toBe('+350 pts');
      expect(formatPoints('1')).toBe('+1 pts');
      expect(formatPoints(300)).toBe('+300 pts');
    });

    it('formats negative values with - and pts', () => {
      expect(formatPoints('-50')).toBe('-50 pts');
      expect(formatPoints(-50)).toBe('-50 pts');
      expect(formatPoints('-9223372036854775808')).toBe(
        '-9223372036854775808 pts',
      );
    });

    it('formats zero values as "0 pts"', () => {
      expect(formatPoints('0')).toBe('0 pts');
      expect(formatPoints('-0')).toBe('0 pts');
      expect(formatPoints(0)).toBe('0 pts');
    });

    it('handles empty or non-numeric strings safely', () => {
      expect(formatPoints('')).toBe('0 pts');
      expect(formatPoints(undefined)).toBe('0 pts');
    });
  });

  describe('getBugUrl', () => {
    it('builds a Buganizer issue URL', () => {
      expect(getBugUrl('12345678')).toBe(
        'https://b.corp.google.com/issues/12345678',
      );
    });

    it('encodes the bug ID', () => {
      expect(getBugUrl('1/2')).toBe('https://b.corp.google.com/issues/1%2F2');
    });
  });

  const getChipLabel = (node: React.ReactNode): string =>
    (node as React.ReactElement<{ label: string }>).props.label;

  describe('extractRuleChips', () => {
    const filterBuilders = {
      model: new StringListFilterCategoryBuilder()
        .setLabel('Model')
        .setOptions([
          { label: 'volteer', value: 'volteer' },
          { label: 'octopus', value: 'octopus' },
        ]),
      dut_state: new StringListFilterCategoryBuilder()
        .setLabel('State')
        .setOptions([
          { label: 'Needs repair', value: 'needs_repair' },
          { label: 'Ready', value: 'ready' },
        ]),
      pools: new StringListFilterCategoryBuilder()
        .setLabel('Pool')
        .setOptions([{ label: 'DUT_POOL_QUOTA', value: 'DUT_POOL_QUOTA' }]),
      pool_health_pct: new RangeFilterCategoryBuilder()
        .setLabel('Pool Health')
        .setMin(0)
        .setMax(100)
        .setScaleFactor(0.01),
    };

    it('extracts chips with native filter serialization using filterBuilders', () => {
      const chips = extractRuleChips(
        'dut_state = "needs_repair"',
        filterBuilders,
      );
      expect(chips).toHaveLength(1);
      expect(getChipLabel(chips[0])).toBe('1 | State IN Needs repair');
    });

    it('extracts exclusion chips with NOT IN serialization', () => {
      const chips = extractRuleChips('dut_state != "ready"', filterBuilders);
      expect(chips).toHaveLength(1);
      expect(getChipLabel(chips[0])).toBe('1 | State NOT IN Ready');
    });

    it('extracts range chips with percentage scaling in native filter format', () => {
      const chips = extractRuleChips('pool_health_pct <= 0.8', filterBuilders);
      expect(chips).toHaveLength(1);
      expect(getChipLabel(chips[0])).toBe('Pool Health <= 80');
    });

    it('extracts compound multi-clause rules as multiple chips', () => {
      const chips = extractRuleChips(
        'model = "octopus" AND pool_health_pct <= 0.8',
        filterBuilders,
      );
      expect(chips).toHaveLength(2);
      expect(getChipLabel(chips[0])).toBe('1 | Model IN octopus');
      expect(getChipLabel(chips[1])).toBe('Pool Health <= 80');
    });

    it('extracts chips for pools filter', () => {
      const chips = extractRuleChips(
        'pools = "DUT_POOL_QUOTA"',
        filterBuilders,
      );
      expect(chips).toHaveLength(1);
      expect(getChipLabel(chips[0])).toBe('1 | Pool IN DUT_POOL_QUOTA');
    });

    it('falls back to raw clause string when no filterBuilders match', () => {
      const chips = extractRuleChips('Rule #rule-9');
      expect(chips).toHaveLength(1);
      expect(getChipLabel(chips[0])).toBe('Rule #rule-9');
    });

    it('returns empty array for empty expression', () => {
      expect(extractRuleChips('')).toEqual([]);
      expect(extractRuleChips('   ')).toEqual([]);
    });
  });

  describe('parsePriorityRules', () => {
    const filterBuilders = {
      model: new StringListFilterCategoryBuilder()
        .setLabel('Model')
        .setOptions([{ label: 'volteer', value: 'volteer' }]),
      pools: new StringListFilterCategoryBuilder()
        .setLabel('Pool')
        .setOptions([{ label: 'QUOTA', value: 'QUOTA' }]),
    };

    it('pre-parses rules and indexes by rule id', () => {
      const rules = [
        { id: 'rule-1', expressionAip160: 'model = "volteer"', weight: '300' },
        { id: 'rule-2', expressionAip160: 'pools = "QUOTA"', weight: '-50' },
      ];

      const rulesMap = parsePriorityRules(rules, filterBuilders);

      expect(rulesMap.size).toBe(2);

      const parsed1 = rulesMap.get('rule-1');
      expect(parsed1).toBeDefined();
      expect(parsed1?.id).toBe('rule-1');
      expect(parsed1?.weight).toBe('300');
      expect(parsed1?.chips).toHaveLength(1);
      expect(getChipLabel(parsed1?.chips[0])).toBe('1 | Model IN volteer');

      const parsed2 = rulesMap.get('rule-2');
      expect(parsed2).toBeDefined();
      expect(parsed2?.id).toBe('rule-2');
      expect(parsed2?.weight).toBe('-50');
      expect(parsed2?.chips).toHaveLength(1);
      expect(getChipLabel(parsed2?.chips[0])).toBe('1 | Pool IN QUOTA');
    });
  });
});
