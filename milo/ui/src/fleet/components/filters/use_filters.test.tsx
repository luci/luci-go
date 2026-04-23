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

import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { MemoryRouter } from 'react-router';

import * as ast from '@/fleet/utils/aip160/ast/ast';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import {
  StringListFilterCategory,
  StringListFilterCategoryBuilder,
} from './string_list_filter';
import {
  useFilters,
  FilterCategoryBuilder,
  FilterCategory,
} from './use_filters';

class MockFilterCategory implements FilterCategory {
  constructor(
    public key: string,
    public label: string,
  ) {}
  toAIP160() {
    return '';
  }
  render() {
    return null;
  }
  getChipLabel() {
    return '';
  }
  isActive() {
    return false;
  }
  clear() {}
  getChildrenSearchScore() {
    return 0;
  }
  setReRender() {}
}

describe('useFilters', () => {
  it('should fallback to normalized key match', () => {
    const mockBuilder: FilterCategoryBuilder<MockFilterCategory> = {
      isFilledIn: () => true,
      build: jest.fn((key, _reRender, _terms) => ({
        isError: false,
        value: new MockFilterCategory(key, key),
        warnings: [],
      })),
    };

    const builders = {
      build: mockBuilder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=labels.%22build%22%3Avalue']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();
    expect(result.current?.filterValues).toBeTruthy();
    expect(mockBuilder.build).toHaveBeenCalled();

    const buildCalls = (mockBuilder.build as jest.Mock).mock.calls;
    expect(buildCalls.length).toBeGreaterThanOrEqual(1);

    const terms = buildCalls[0][2];
    expect(terms).toBeTruthy();
    expect(terms.length).toBe(1);
    // The term should be passed to the builder even though the URL used labels."build"
    // and the builder key is "build".
    const arg = terms[0].simple.arg;
    expect(arg).toBeTruthy();
    expect(arg.kind).toBe('Comparable');
    expect((arg as ast.Comparable).member.value.value).toBe('value');
  });

  it('should fallback to getFilters for legacy URLs', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([
        { label: 'v1', value: 'v1' },
        { label: 'v2', value: 'v2' },
      ]);

    const builders = {
      model: builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/?filters=model+%3D+%28%22v1%22+OR+%22v2%22%29']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());
    const filters = result.current.filterValues;

    const category = filters?.['model'];
    expect(category).toBeInstanceOf(StringListFilterCategory);

    expect((category as StringListFilterCategory).getSelectedOptions()).toEqual(
      ['v1', 'v2'],
    );
  });

  it('should not update URL when filters are stable', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([
        { label: 'v1', value: 'v1' },
        { label: 'v2', value: 'v2' },
      ]);

    const builders = {
      model: builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/?filters=model+%3D+%28%22v1%22+OR+%22v2%22%29']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const initialAip160 = result.current.getAip160String();

    result.current.getAip160String();

    expect(result.current.getAip160String()).toEqual(initialAip160);
  });

  it('should return stable AIP-160 string with deterministic category order', async () => {
    const builderHostGroup = new StringListFilterCategoryBuilder()
      .setLabel('Host Group')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builderLabName = new StringListFilterCategoryBuilder()
      .setLabel('Lab Name')
      .setOptions([{ label: 'v2', value: 'v2' }]);

    const builders = {
      host_group: builderHostGroup,
      lab_name: builderLabName,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/?filters=lab_name+%3D+%22v2%22+AND+host_group+%3D+%22v1%22',
        ]}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const generated = result.current.getAip160String();
    expect(generated).toEqual('host_group = ("v1") AND lab_name = ("v2")');
  });
});
