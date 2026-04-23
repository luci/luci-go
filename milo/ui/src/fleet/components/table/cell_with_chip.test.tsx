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

import { SettingsProvider } from '@/fleet/context/providers';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { renderChipCell, StateUnion } from './cell_with_chip';

describe('renderChipCell', () => {
  const mockGetColor = jest.fn((value: StateUnion) => {
    if ((value as string) === 'ready') return 'green';
    return 'red';
  });

  const mockGetValueOrUrl = jest.fn((value: string) => `/test/${value}`);

  afterEach(() => {
    jest.clearAllMocks();
  });

  test('should render properly using MRT_Cell (Material React Table)', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const CellComponent = renderChipCell<any>(
      mockGetValueOrUrl,
      mockGetColor,
      undefined,
      true,
    );

    const mrtProps = {
      cell: {
        getValue: () => 'repair_failed',
      },
      row: {
        original: { id: '2' },
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <CellComponent {...mrtProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const chip = screen.getByText('repair_failed');
    expect(chip.closest('a')).toHaveAttribute('href', '/test/repair_failed');
    expect(mockGetColor).toHaveBeenCalledWith('repair_failed');
  });

  test('should prioritize overrideValue over cell or grid values', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const CellComponent = renderChipCell<any>(
      mockGetValueOrUrl,
      mockGetColor,
      undefined,
      true,
      'needs_repair' as StateUnion,
    );

    const mrtProps = {
      cell: {
        getValue: () => 'repair_failed', // This should be ignored
      },
      row: {
        original: { id: '3' },
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any;

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <CellComponent {...mrtProps} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const chip = screen.getByText('needs_repair');
    // Use the overrideValue instead of the cell value
    expect(chip.closest('a')).toHaveAttribute('href', '/test/needs_repair');
    expect(mockGetColor).toHaveBeenCalledWith('needs_repair');
  });
});
