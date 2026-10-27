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

import { render, screen } from '@testing-library/react';

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
    expect(category.getChipLabel()).toBe('10 <= Count <= 50');
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

  it('should parse decimal values from dotted AST Member fields', () => {
    const termWithDottedAst: ast.Term & { simple: ast.Restriction } = {
      kind: 'Term',
      negated: false,
      simple: {
        kind: 'Restriction',
        comparable: {
          kind: 'Comparable',
          member: {
            kind: 'Member',
            value: { kind: 'Value', value: 'pool_health_pct', quoted: false },
            fields: [],
          },
        },
        comparator: '<=',
        arg: {
          kind: 'Comparable',
          member: {
            kind: 'Member',
            value: { kind: 'Value', value: '0', quoted: false },
            fields: [{ kind: 'Value', value: '8', quoted: false }],
          },
        },
      },
    };

    const builder = new RangeFilterCategoryBuilder()
      .setLabel('Pool Health')
      .setMin(0)
      .setMax(100)
      .setScaleFactor(0.01);

    const result = builder.build('pool_health_pct', () => {}, [
      termWithDottedAst,
    ]);
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    // 0.8 / 0.01 = 80
    expect(category.value.max).toBe(80);
    expect(category.toAIP160()).toBe('pool_health_pct <= 0.8');
    expect(category.getChipLabel()).toBe('Pool Health <= 80');
  });

  it('should expose utilization filter constants with expected values', () => {
    expect(UTILIZATION_FILTER_KEYS).toContain('average_7d');
    expect(UTILIZATION_FILTER_KEYS).toContain('average_30d');
    expect(DEFAULT_UTILIZATION_SCALE_FACTOR).toBe(0.01);
  });

  describe('getChipLabel with unit', () => {
    it('should format chip label with unit when both min and max are set', () => {
      const result = RangeFilterCategory.create(
        'Utilization',
        'utilization',
        0,
        100,
        jest.fn(),
        null,
        1,
        '%',
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { min: 10, max: 90 };
      expect(category.getChipLabel()).toBe('10% <= Utilization <= 90%');
    });

    it('should format chip label with unit when min equals max', () => {
      const result = RangeFilterCategory.create(
        'Utilization',
        'utilization',
        0,
        100,
        jest.fn(),
        null,
        1,
        '%',
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { min: 50, max: 50 };
      expect(category.getChipLabel()).toBe('Utilization = 50%');
    });

    it('should format chip label with unit when only min is set', () => {
      const result = RangeFilterCategory.create(
        'Utilization',
        'utilization',
        0,
        100,
        jest.fn(),
        null,
        1,
        '%',
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { min: 25 };
      expect(category.getChipLabel()).toBe('Utilization >= 25%');
    });

    it('should format chip label with unit when only max is set', () => {
      const result = RangeFilterCategory.create(
        'Utilization',
        'utilization',
        0,
        100,
        jest.fn(),
        null,
        1,
        '%',
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { max: 75 };
      expect(category.getChipLabel()).toBe('Utilization <= 75%');
    });

    it('should format chip label without unit when unit is not provided', () => {
      const result = RangeFilterCategory.create(
        'Slippage',
        'slippage',
        0,
        1000,
        jest.fn(),
        null,
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { min: 5, max: 10 };
      expect(category.getChipLabel()).toBe('5 <= Slippage <= 10');
    });
  });

  describe('render', () => {
    it('should render RangeFilter with unit text', () => {
      const result = RangeFilterCategory.create(
        'Utilization',
        'utilization',
        0,
        100,
        jest.fn(),
        null,
        1,
        '%',
      );
      expect(result.isError).toBe(false);
      if (result.isError) return;

      const category = result.value;
      category.value = { min: 20, max: 80 };

      render(
        category.render(
          '',
          jest.fn(),
          jest.fn(),
          jest.fn(),
        ) as React.ReactElement,
      );

      expect(screen.getByText('%')).toBeInTheDocument();
      expect(screen.getByLabelText('Min')).toBeInTheDocument();
      expect(screen.getByLabelText('Max')).toBeInTheDocument();
    });
  });
});

describe('RangeFilterCategoryBuilder', () => {
  it('should build a RangeFilterCategory without unit', () => {
    const builder = new RangeFilterCategoryBuilder()
      .setLabel('Utilization')
      .setMin(0)
      .setMax(100);

    expect(builder.isFilledIn()).toBe(true);

    const result = builder.build('utilization', jest.fn(), null);
    expect(result.isError).toBe(false);
    if (result.isError) return;

    expect(result.value.label).toBe('Utilization');
    expect(result.value.key).toBe('utilization');
    expect(result.value.min).toBe(0);
    expect(result.value.max).toBe(100);
    expect(result.value.unit).toBeUndefined();
  });

  it('should build a RangeFilterCategory with unit', () => {
    const builder = new RangeFilterCategoryBuilder()
      .setLabel('7 Day Average Utilization')
      .setMin(0)
      .setMax(100)
      .setUnit('%');

    expect(builder.isFilledIn()).toBe(true);

    const result = builder.build('average_7d', jest.fn(), null);
    expect(result.isError).toBe(false);
    if (result.isError) return;

    expect(result.value.label).toBe('7 Day Average Utilization');
    expect(result.value.key).toBe('average_7d');
    expect(result.value.min).toBe(0);
    expect(result.value.max).toBe(100);
    expect(result.value.unit).toBe('%');
  });

  it('should return error if builder is not filled in', () => {
    const builder = new RangeFilterCategoryBuilder().setLabel('Utilization');
    expect(builder.isFilledIn()).toBe(false);

    const result = builder.build('utilization', jest.fn(), null);
    expect(result.isError).toBe(true);
  });

  it('should support helperText on builder and render it in option component', () => {
    const builder = new RangeFilterCategoryBuilder()
      .setLabel('Pool Health')
      .setMin(0)
      .setMax(100)
      .setScaleFactor(0.01)
      .setHelperText('* Note: Claimed devices count as healthy');

    const result = builder.build('pool_health_pct', () => {}, null);
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;
    expect(category.helperText).toBe(
      '* Note: Claimed devices count as healthy',
    );

    const { getByText } = render(
      category.render(
        '',
        () => {},
        () => {},
        () => {},
      ) as React.ReactElement,
    );
    expect(
      getByText('* Note: Claimed devices count as healthy'),
    ).toBeInTheDocument();
  });
});
