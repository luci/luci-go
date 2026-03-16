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

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FilterCategory } from '../filters/use_filters';

import { FilterBar } from './filter_bar';

const createMockFilterCategory = (
  value: string,
  label: string,
  isActive = false,
): FilterCategory =>
  ({
    value,
    label,
    isActive: () => isActive,
    renderChip: () => <div key={value}>{label}</div>,
    render: () => <div key={`menu-${value}`}>{label} Menu</div>,
    clear: jest.fn(),
    getChildrenSearchScore: () => 0,
  }) as unknown as FilterCategory;

describe('FilterBar', () => {
  it('should render the search bar', () => {
    const filterCategoryDatas = [
      createMockFilterCategory('option1', 'Option 1'),
    ];
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <FilterBar
            filterCategoryDatas={filterCategoryDatas}
            onApply={jest.fn()}
          />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByPlaceholderText(/Add a filter/)).toBeInTheDocument();
  });
});
