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

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

type UserEventInstance = ReturnType<typeof userEvent.setup>;

const matchesHeader = (
  headerText: string,
  expected: string | RegExp,
): boolean => {
  if (typeof expected === 'string') {
    return headerText.trim() === expected;
  }
  return expected.test(headerText.trim());
};

/**
 * Shared test driver for Material React Table / `useFCDataTable` views across
 * all Fleet Console pages. Encapsulates row lookup, header-indexed cell
 * assertions, and cursor pagination URLs so domain test helpers stay focused
 * on business behavior rather than DOM table structure.
 */
export const fleetTable = {
  /**
   * Builds the canonical Fleet pager URL for a 1-indexed page number and page
   * size using the `limit` and `cursor` query parameters read by `usePager`.
   */
  pageUrl(pageNumber: number, pageSize: number): string {
    const offset = Math.max(0, pageNumber - 1) * pageSize;
    const params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (offset > 0) {
      params.set('cursor', `token-${offset}`);
    }
    return `/?${params.toString()}`;
  },

  /**
   * Finds the `HTMLTableRowElement` containing `text` and throws a clear error
   * if the matched element is not inside a table row.
   */
  async findRowByText(text: string | RegExp): Promise<HTMLTableRowElement> {
    const element = await screen.findByText(text);
    const row = element.closest('tr');
    if (!row) {
      throw new Error(
        `Element matching ${String(text)} was found, but is not inside a <tr> row.`,
      );
    }
    return row;
  },

  /**
   * Resolves a cell within `row` by matching the visible column header text,
   * avoiding brittle hardcoded column indices (`row.cells[0]`).
   */
  getCell(
    row: HTMLTableRowElement,
    columnHeader: string | RegExp,
  ): HTMLTableCellElement {
    const table = row.closest('table');
    const headers = table
      ? within(table).getAllByRole('columnheader')
      : screen.getAllByRole('columnheader');
    const colIndex = headers.findIndex((header) =>
      matchesHeader(header.textContent ?? '', columnHeader),
    );
    if (colIndex === -1) {
      const available = headers.map((h) => h.textContent?.trim()).join(', ');
      throw new Error(
        `Column header matching ${String(columnHeader)} not found. Available headers: [${available}]`,
      );
    }
    const cell = row.cells.item(colIndex);
    if (!cell) {
      throw new Error(
        `Cell at index ${colIndex} (${String(columnHeader)}) does not exist in row.`,
      );
    }
    return cell;
  },

  /**
   * Asserts that the cell under `columnHeader` in `row` matches `expected`.
   */
  expectCellText(
    row: HTMLTableRowElement,
    columnHeader: string | RegExp,
    expected: string | RegExp,
  ) {
    const cell = fleetTable.getCell(row, columnHeader);
    expect(cell).toHaveTextContent(expected);
  },

  /**
   * Asserts the ordered list of visible column headers.
   */
  expectColumns(expectedHeaders: string[], container?: HTMLElement) {
    const scope = container ? within(container) : screen;
    const columnHeaders = scope.getAllByRole('columnheader');
    expect(columnHeaders.map((h) => h.textContent)).toEqual(expectedHeaders);
  },

  /**
   * Asserts that the table renders its empty state placeholder.
   */
  async expectEmpty(emptyMessage: string | RegExp = 'No records to display') {
    expect(await screen.findByText(emptyMessage)).toBeInTheDocument();
  },

  /**
   * Clicks the next page button in `<FleetBottomToolbar />`.
   */
  async goToNextPage(user: UserEventInstance) {
    const nextButton = await screen.findByRole('button', {
      name: /next page/i,
    });
    await user.click(nextButton);
  },

  /**
   * Clicks the previous page button in `<FleetBottomToolbar />`.
   */
  async goToPreviousPage(user: UserEventInstance) {
    const prevButton = await screen.findByRole('button', {
      name: /previous page/i,
    });
    await user.click(prevButton);
  },

  /**
   * Returns a scoped query helper for interactions inside a specific table row.
   */
  withinRow(row: HTMLTableRowElement) {
    return within(row);
  },
};
