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

import { StringListFilterCategory } from './string_list_filter';

describe('StringListFilterCategory', () => {
  it('should get selected options', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);
  });

  it('should set selected options correctly with quotes', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);
  });

  it('should handle unquoted keys for unquoted options', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: 'nexus', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['pixel']);
  });
  it('should handle internal quotes in keys', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [{ value: '"device \\"pro\\" model"', label: 'Pro Model' }],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['device \\"pro\\" model']);
    expect(category.getSelectedOptions()).toEqual(['"device \\"pro\\" model"']);
  });

  it('should clear all options when an empty array is passed', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);

    category.setSelectedOptions([]);
    expect(category.getSelectedOptions()).toEqual([]);
  });

  it('should generate correct AIP-160 string when excluded', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: 'nexus', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel']);
    category.isExcluded = true;
    expect(category.toAIP160()).toEqual('model != "pixel"');

    category.setSelectedOptions(['pixel', 'nexus']);
    expect(category.toAIP160()).toEqual(
      'model != "pixel" AND model != "nexus"',
    );
  });

  it('should parse negated filter string correctly', () => {
    const mockTerm = {
      kind: 'Term',
      negated: true,
      simple: {
        kind: 'Restriction',
        comparator: '=',
        arg: {
          kind: 'Comparable',
          member: {
            value: { value: 'pixel', quoted: false },
            fields: [],
          },
        },
      },
    } as unknown as ast.Term & { simple: ast.Restriction };

    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: 'nexus', label: 'Nexus' },
      ],
      [],
      () => {},
      [mockTerm],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.isExcluded).toBe(true);
    expect(category.getSelectedOptions()).toEqual(['pixel']);
  });

  it('should generate correct AIP-160 string for Blank', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: '(Blank)', label: 'Blank' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['(Blank)']);
    expect(category.toAIP160()).toEqual('NOT model:*');

    category.isExcluded = true;
    expect(category.toAIP160()).toEqual('model:*');
  });

  it('should parse Blank filter in Exclude mode correctly', () => {
    const mockTerm = {
      kind: 'Term',
      negated: false,
      simple: {
        kind: 'Restriction',
        comparator: ':',
        arg: {
          kind: 'Comparable',
          member: {
            value: { value: '*', quoted: false },
            fields: [],
          },
        },
      },
    } as unknown as ast.Term & { simple: ast.Restriction };

    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: '(Blank)', label: 'Blank' },
      ],
      [],
      () => {},
      [mockTerm],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    expect(category.isExcluded).toBe(true);
    expect(category.getSelectedOptions()).toEqual(['(Blank)']);
  });

  it('should set selected options case-insensitively', () => {
    const result = StringListFilterCategory.create(
      'Dut State',
      'dut_state',
      [
        { value: '"ready"', label: 'Ready' },
        { value: '"repair_failed"', label: 'Repair Failed' },
      ],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['READY', 'REPAIR_FAILED']);
    expect(category.getSelectedOptions()).toEqual([
      '"ready"',
      '"repair_failed"',
    ]);
  });

  it('should quote values containing colons', () => {
    const result = StringListFilterCategory.create(
      'Host Group',
      'host_group',
      [{ value: 'atc:crystalball', label: 'Crystalball' }],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['atc:crystalball']);
    expect(category.toAIP160()).toEqual('(host_group = "atc:crystalball")');
  });

  it('should quote values containing spaces', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [{ value: 'pixel pro', label: 'Pixel Pro' }],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel pro']);
    expect(category.toAIP160()).toEqual('(model = "pixel pro")');
  });

  it('should quote values containing parentheses', () => {
    const result = StringListFilterCategory.create(
      'Model',
      'model',
      [{ value: 'pixel(gen1)', label: 'Pixel Gen1' }],
      [],
      () => {},
      [],
    );
    expect(result.isError).toBe(false);
    if (result.isError) return;
    const category = result.value;

    category.setSelectedOptions(['pixel(gen1)']);
    expect(category.toAIP160()).toEqual('(model = "pixel(gen1)")');
  });
});
