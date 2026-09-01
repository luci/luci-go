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

import { FC_CellProps } from '@/fleet/types/table';

import { renderTimestampCell } from './cell_with_timestamp';

describe('renderTimestampCell', () => {
  it('returns dash fallback when value is empty, null, or undefined', () => {
    const mockCellPropsNull = {
      cell: { getValue: () => null },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    const mockCellPropsUndefined = {
      cell: { getValue: () => undefined },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    const mockCellPropsEmpty = {
      cell: { getValue: () => '' },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    const mockCellPropsEmptyArray = {
      cell: { getValue: () => [] },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    expect(renderTimestampCell(mockCellPropsNull)).toBe('-');
    expect(renderTimestampCell(mockCellPropsUndefined)).toBe('-');
    expect(renderTimestampCell(mockCellPropsEmpty)).toBe('-');
    expect(renderTimestampCell(mockCellPropsEmptyArray)).toBe('-');
  });

  it('renders raw string fallback when ISO string is invalid', () => {
    const mockCellPropsInvalid = {
      cell: { getValue: () => 'not-a-valid-date' },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    render(<>{renderTimestampCell(mockCellPropsInvalid)}</>);
    expect(screen.getByText('not-a-valid-date')).toBeInTheDocument();
  });

  it('supports TimestampDisplayProps object format directly', () => {
    expect(renderTimestampCell({ value: null })).toBe('-');
    expect(renderTimestampCell({ value: undefined })).toBe('-');
    expect(renderTimestampCell({ value: '' })).toBe('-');

    render(<>{renderTimestampCell({ value: 'invalid-date' })}</>);
    expect(screen.getByText('invalid-date')).toBeInTheDocument();
  });

  it('handles array values by taking the first ISO string element', () => {
    const mockCellPropsArray = {
      cell: { getValue: () => ['2026-08-01T12:00:00.000Z'] },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    const { container } = render(
      <>{renderTimestampCell(mockCellPropsArray)}</>,
    );
    expect(container).not.toBeEmptyDOMElement();
  });

  it('renders SmartRelativeTimestamp component when given a valid ISO string', () => {
    const mockCellPropsValid = {
      cell: { getValue: () => '2026-08-01T12:00:00.000Z' },
    } as unknown as FC_CellProps<Record<string, unknown>>;

    const { container } = render(
      <>{renderTimestampCell(mockCellPropsValid)}</>,
    );
    expect(container).not.toBeEmptyDOMElement();
  });
});
