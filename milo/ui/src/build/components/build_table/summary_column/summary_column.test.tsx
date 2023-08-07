// Copyright 2023 The LUCI Authors.
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

import { TableCell } from '@mui/material';
import { act, cleanup, render, screen } from '@testing-library/react';

import { Build, BuildStatus } from '@/common/services/buildbucket';
import {
  ExpandableEntriesState,
  ExpandableEntriesStateInstance,
} from '@/common/store/expandable_entries_state';

import { RowStateProvider, TableStateProvider } from '../context';

import { SummaryContentCell } from './summary_column';

jest.mock('@mui/material', () =>
  self.createSelectiveSpiesFromModule<typeof import('@mui/material')>(
    '@mui/material',
    ['TableCell']
  )
);

const buildWithSummary = {
  id: '1234',
  status: BuildStatus.Success,
  summaryMarkdown: '<strong>build is fine</strong>',
} as Build;

const buildWithNoSummary = {
  id: '2345',
  status: BuildStatus.Success,
} as Build;

describe('SummaryContentCell', () => {
  let tableCellSpy: jest.MockedFunctionDeep<typeof TableCell>;

  beforeEach(() => {
    jest.useFakeTimers();
    tableCellSpy = jest
      .mocked(TableCell)
      .mockImplementation(({ children }) => <td>{children}</td>);
  });

  afterEach(() => {
    jest.useRealTimers();
    tableCellSpy.mockReset();
  });

  describe('when there is summary', () => {
    let tableState: ExpandableEntriesStateInstance;

    beforeEach(() => {
      tableState = ExpandableEntriesState.create({
        defaultExpanded: false,
      });
      render(
        <TableStateProvider value={tableState}>
          <table>
            <tbody>
              <RowStateProvider value={buildWithSummary}>
                <tr>
                  <SummaryContentCell />
                </tr>
              </RowStateProvider>
            </tbody>
          </table>
        </TableStateProvider>
      );
    });

    afterEach(() => {
      cleanup();
    });

    it('should expand/collapse correctly', async () => {
      const toggleButton = screen.getByLabelText('toggle-row');

      expect(toggleButton).not.toBeDisabled();
      expect(toggleButton).not.toHaveStyleRule('visibility', 'hidden');

      expect(
        toggleButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();

      // Expand by clicking on toggle button.
      act(() => toggleButton.click());
      expect(
        toggleButton.querySelector("[data-testid='ChevronRightIcon']")
      ).not.toBeInTheDocument();
      expect(
        toggleButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).toBeInTheDocument();

      // Collapse by clicking on toggle button.
      act(() => toggleButton.click());
      expect(
        toggleButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();

      // Expand again by changing the default state.
      act(() => tableState.toggleAll(true));
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleButton.querySelector("[data-testid='ChevronRightIcon']")
      ).not.toBeInTheDocument();
      expect(
        toggleButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).toBeInTheDocument();

      // Collapse by clicking on toggle button.
      act(() => tableState.toggleAll(false));
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();
    });

    it('should avoid unnecessary rerendering', async () => {
      const toggleButton = screen.getByLabelText('toggle-row');
      expect(tableCellSpy).toHaveBeenCalledTimes(1);

      // Expand by clicking on toggle button.
      act(() => toggleButton.click());
      expect(tableCellSpy).toHaveBeenCalledTimes(2);

      // Expand by changing the default state.
      act(() => tableState.toggleAll(true));
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. The entry was expanded already.
      expect(tableCellSpy).toHaveBeenCalledTimes(2);

      // Collapse by clicking on toggle button.
      act(() => toggleButton.click());
      expect(tableCellSpy).toHaveBeenCalledTimes(3);

      // Collapse by clicking on toggle button.
      act(() => tableState.toggleAll(false));
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. The entry was collapsed already.
      expect(tableCellSpy).toHaveBeenCalledTimes(3);
    });
  });

  describe('when there is no summary', () => {
    let tableState: ExpandableEntriesStateInstance;

    beforeEach(() => {
      tableState = ExpandableEntriesState.create({
        defaultExpanded: false,
      });
      render(
        <TableStateProvider value={tableState}>
          <table>
            <tbody>
              <RowStateProvider value={buildWithNoSummary}>
                <tr>
                  <SummaryContentCell />
                </tr>
              </RowStateProvider>
            </tbody>
          </table>
        </TableStateProvider>
      );
    });

    afterEach(() => {
      cleanup();
    });

    it('should disable toggle button', async () => {
      const toggleButton = screen.getByLabelText('toggle-row');

      expect(toggleButton).toBeDisabled();
      expect(toggleButton).toHaveStyleRule('visibility', 'hidden');
    });

    it('should avoid unnecessary rerendering', async () => {
      expect(tableCellSpy).toHaveBeenCalledTimes(1);

      // Expand by changing the default state.
      act(() => tableState.toggleAll(true));
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There is no summary anyway.
      expect(tableCellSpy).toHaveBeenCalledTimes(1);

      // Collapse by clicking on toggle button.
      act(() => tableState.toggleAll(false));
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There is no summary anyway.
      expect(tableCellSpy).toHaveBeenCalledTimes(1);
    });
  });
});
