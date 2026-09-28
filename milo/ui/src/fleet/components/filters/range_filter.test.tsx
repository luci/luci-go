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

import * as ast from '@/fleet/utils/aip160/ast/ast';

import {
  DEFAULT_UTILIZATION_SCALE_FACTOR,
  RangeFilterCategory,
  RangeFilterCategoryBuilder,
  UTILIZATION_FILTER_KEYS,
} from './range_filter';

const createComparableTerm = (
  field: string,
  comparator: string,
  val: string,
): ast.Term & { simple: ast.Restriction } => ({
  kind: 'Term',
  negated: false,
  simple: {
    kind: 'Restriction',
    comparable: {
      kind: 'Comparable',
      member: {
        kind: 'Member',
        value: { kind: 'Value', value: field, quoted: false },
        fields: [],
      },
    },
    comparator,
    arg: {
      kind: 'Comparable',
      member: {
        kind: 'Member',
        value: { kind: 'Value', value: val, quoted: false },
        fields: [],
      },
    },
  },
});

describe('RangeFilterCategory', () => {
  it('should initialize with empty value and empty AIP160', () => {
    const result = RangeFilterCategory.create(
      'Utilization',
      'utilization',
      0,
      100,
      () => {},
      null,
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.isActive()).toBe(false);
    expect(category.toAIP160()).toBe('');
  });

  it('should generate AIP160 with standard integer bounds', () => {
    const result = RangeFilterCategory.create(
      'Count',
      'count',
      0,
      1000,
      () => {},
      [
        createComparableTerm('count', '>=', '10'),
        createComparableTerm('count', '<=', '50'),
      ],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.value).toEqual({ min: 10, max: 50 });
    expect(category.toAIP160()).toBe('count >= 10 AND count <= 50');
    expect(category.getChipLabel()).toBe('[ Count ]: from 10 to 50');
  });

  it('should automatically scale utilization filters (0.01 scale factor) for average_7d', () => {
    // When incoming terms from AIP-160 are decimals (e.g. backend scaled 0.25 to 0.75)
    const result = RangeFilterCategory.create(
      '7 Day Average Utilization',
      '"average_7d"',
      0,
      100,
      () => {},
      [
        createComparableTerm('average_7d', '>=', '0.25'),
        createComparableTerm('average_7d', '<=', '0.75'),
      ],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    // Converted to UI percentage (0-100)
    expect(category.value).toEqual({ min: 25, max: 75 });
    // Scaled back down for AIP-160
    expect(category.toAIP160()).toBe(
      '"average_7d" >= 0.25 AND "average_7d" <= 0.75',
    );
  });

  it('should handle already-unscaled integer input in URL for average_30d', () => {
    // If user typed 25 directly in URL instead of 0.25
    const result = RangeFilterCategory.create(
      '30 Day Average Utilization',
      '"average_30d"',
      0,
      100,
      () => {},
      [
        createComparableTerm('average_30d', '>=', '20'),
        createComparableTerm('average_30d', '<=', '80'),
      ],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    // Stays 20 to 80 because 20 > 100 * 0.01
    expect(category.value).toEqual({ min: 20, max: 80 });
    expect(category.toAIP160()).toBe(
      '"average_30d" >= 0.2 AND "average_30d" <= 0.8',
    );
  });

  it('should support decimal inputs without truncation', () => {
    const result = RangeFilterCategory.create(
      'Score',
      'score',
      0,
      10,
      () => {},
      [
        createComparableTerm('score', '>=', '2.5'),
        createComparableTerm('score', '<=', '7.85'),
      ],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.value).toEqual({ min: 2.5, max: 7.85 });
    expect(category.toAIP160()).toBe('score >= 2.5 AND score <= 7.85');
  });

  it('should support RangeFilterCategoryBuilder with setScaleFactor', () => {
    const builder = new RangeFilterCategoryBuilder()
      .setLabel('Custom Scale')
      .setMin(0)
      .setMax(100)
      .setScaleFactor(0.01);

    expect(builder.isFilledIn()).toBe(true);

    const result = builder.build('custom_field', () => {}, [
      createComparableTerm('custom_field', '>=', '0.5'),
    ]);
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.value.min).toBe(50);
    expect(category.toAIP160()).toBe('custom_field >= 0.5');
  });

  it('should expose utilization filter constants with expected values', () => {
    expect(UTILIZATION_FILTER_KEYS).toContain('average_7d');
    expect(UTILIZATION_FILTER_KEYS).toContain('average_30d');
    expect(DEFAULT_UTILIZATION_SCALE_FACTOR).toBe(0.01);
  });
});
