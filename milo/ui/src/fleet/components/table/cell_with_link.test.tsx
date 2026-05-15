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
import { MRT_RowData } from 'material-react-table';

import { FC_CellProps } from '@/fleet/types/table';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { renderCellWithLink } from './cell_with_link';

describe('renderCellWithLink', () => {
  const mockLinkGenerator = jest.fn((value: string) => `/test/${value}`);

  afterEach(() => {
    jest.clearAllMocks();
  });

  test('should render a single link for a single value', () => {
    const CellComponent = renderCellWithLink<MRT_RowData>({
      linkGenerator: mockLinkGenerator,
    });

    const mrtProps = {
      cell: {
        getValue: () => 'single_dut',
      },
      row: {
        original: { id: '1' },
      },
      column: { id: 'test_col' },
    } as unknown as FC_CellProps<MRT_RowData>;

    render(
      <FakeContextProvider>
        <CellComponent {...mrtProps} />
      </FakeContextProvider>,
    );

    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute('href', '/test/single_dut');
    expect(links[0]).toHaveTextContent('single_dut');
    expect(mockLinkGenerator).toHaveBeenCalledWith('single_dut', { id: '1' });
  });

  test('should render separate links sorted by length descending and alphabetically when given an array of strings directly', () => {
    const CellComponent = renderCellWithLink<MRT_RowData>({
      linkGenerator: mockLinkGenerator,
    });

    const mrtProps = {
      cell: {
        getValue: () => ['dut1', 'dut2,with,comma', 'dut3'],
      },
      row: {
        original: { id: '2' },
      },
      column: { id: 'test_col' },
    } as unknown as FC_CellProps<MRT_RowData>;

    const { container } = render(
      <FakeContextProvider>
        <CellComponent {...mrtProps} />
      </FakeContextProvider>,
    );

    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(3);
    // 'dut2,with,comma' is longest (length 15), so it comes first. 'dut1' and 'dut3' tie on length 4, sorted alphabetically.
    expect(links[0]).toHaveAttribute('href', '/test/dut2,with,comma');
    expect(links[0]).toHaveTextContent('dut2,with,comma');
    expect(links[1]).toHaveAttribute('href', '/test/dut1');
    expect(links[1]).toHaveTextContent('dut1');
    expect(links[2]).toHaveAttribute('href', '/test/dut3');
    expect(links[2]).toHaveTextContent('dut3');

    expect(mockLinkGenerator).toHaveBeenCalledWith('dut1', { id: '2' });
    expect(mockLinkGenerator).toHaveBeenCalledWith('dut2,with,comma', {
      id: '2',
    });
    expect(mockLinkGenerator).toHaveBeenCalledWith('dut3', { id: '2' });

    expect(container).toHaveTextContent('dut2,with,comma, dut1, dut3');
  });
});
