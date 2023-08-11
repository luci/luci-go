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

import { act, cleanup, render, screen } from '@testing-library/react';

import { Build, BuildStatus } from '@/common/services/buildbucket';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildTable } from '../build_table';
import { BuildTableBody } from '../build_table_body';
import { BuildTableHead } from '../build_table_head';
import { BuildTableRow } from '../build_table_row';
import { useRowState } from '../context';

import { SummaryContentCell, SummaryHeadCell } from './summary_column';

jest.mock('../context', () =>
  self.createSelectiveSpiesFromModule<
    typeof import('@/build/components/build_table/context')
  >('@/build/components/build_table/context', ['useRowState'])
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
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  describe('when there is summary', () => {
    let useRowStateSpy: jest.MockedFunctionDeep<typeof useRowState>;

    beforeEach(() => {
      useRowStateSpy = jest.mocked(useRowState);
      render(
        <FakeContextProvider>
          <BuildTable>
            <BuildTableHead>
              <SummaryHeadCell />
            </BuildTableHead>
            <BuildTableBody>
              <BuildTableRow build={buildWithSummary}>
                <SummaryContentCell />
              </BuildTableRow>
            </BuildTableBody>
          </BuildTable>
        </FakeContextProvider>
      );
    });

    afterEach(() => {
      useRowStateSpy.mockClear();
      cleanup();
    });

    it('should expand/collapse correctly', async () => {
      const toggleRowButton = screen.getByLabelText('toggle-row');
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');

      expect(toggleRowButton).not.toBeDisabled();
      expect(toggleRowButton).not.toHaveStyleRule('visibility', 'hidden');

      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();

      // Expand by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']")
      ).not.toBeInTheDocument();
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).toBeInTheDocument();

      // Collapse by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();

      // Expand again by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']")
      ).not.toBeInTheDocument();
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).toBeInTheDocument();

      // Collapse again by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']")
      ).toBeInTheDocument();
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']")
      ).not.toBeInTheDocument();
    });

    it('should avoid unnecessary rerendering', async () => {
      const toggleRowButton = screen.getByLabelText('toggle-row');
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');
      expect(useRowStateSpy).toHaveBeenCalledTimes(1);

      // Expand by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(useRowStateSpy).toHaveBeenCalledTimes(2);

      // Expand by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. The entry was expanded already.
      expect(useRowStateSpy).toHaveBeenCalledTimes(2);

      // Collapse by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(useRowStateSpy).toHaveBeenCalledTimes(3);

      // Collapse by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. The entry was collapsed already.
      expect(useRowStateSpy).toHaveBeenCalledTimes(3);
    });
  });

  describe('when there is no summary', () => {
    let useRowStateSpy: jest.MockedFunctionDeep<typeof useRowState>;

    beforeEach(() => {
      useRowStateSpy = jest.mocked(useRowState);
      render(
        <FakeContextProvider>
          <BuildTable>
            <BuildTableHead>
              <SummaryHeadCell />
            </BuildTableHead>
            <BuildTableBody>
              <BuildTableRow build={buildWithNoSummary}>
                <SummaryContentCell />
              </BuildTableRow>
            </BuildTableBody>
          </BuildTable>
        </FakeContextProvider>
      );
    });

    afterEach(() => {
      useRowStateSpy.mockClear();
      cleanup();
    });

    it('should disable toggle button', async () => {
      const toggleButton = screen.getByLabelText('toggle-row');

      expect(toggleButton).toBeDisabled();
      expect(toggleButton).toHaveStyleRule('visibility', 'hidden');
    });

    it('should avoid unnecessary rerendering', async () => {
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');
      expect(useRowStateSpy).toHaveBeenCalledTimes(1);

      // Expand by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There is no summary anyway.
      expect(useRowStateSpy).toHaveBeenCalledTimes(1);

      // Collapse by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There is no summary anyway.
      expect(useRowStateSpy).toHaveBeenCalledTimes(1);
    });
  });
});
