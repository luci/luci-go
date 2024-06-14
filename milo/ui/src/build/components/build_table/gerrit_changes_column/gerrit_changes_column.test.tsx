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

import { cleanup, render, screen } from '@testing-library/react';
import { act } from 'react';

import { OutputBuild } from '@/build/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildTable } from '../build_table';
import { BuildTableBody } from '../build_table_body';
import { BuildTableHead } from '../build_table_head';
import { BuildTableRow } from '../build_table_row';
import { useBuild } from '../context';

import {
  GerritChangesContentCell,
  GerritChangesHeadCell,
} from './gerrit_changes_column';

jest.mock('../context', () =>
  self.createSelectiveSpiesFromModule<typeof import('../context')>(
    '../context',
    ['useBuild'],
  ),
);

const buildWithMultipleCls = Build.fromPartial({
  id: '1234',
  input: {
    gerritChanges: [
      {
        host: 'gerrit-host',
        project: 'proj',
        change: 'cl1',
        patchset: '1',
      },
      {
        host: 'gerrit-host',
        project: 'proj',
        change: 'cl2',
        patchset: '1',
      },
      {
        host: 'gerrit-host',
        project: 'proj',
        change: 'cl3',
        patchset: '1',
      },
    ],
  },
}) as OutputBuild;

const buildWithNoCls = {
  id: '2345',
  input: {},
} as OutputBuild;

describe('GerritChangesContentCell', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  describe('when there are CLs', () => {
    let useBuildSpy: jest.MockedFunctionDeep<typeof useBuild>;
    beforeEach(() => {
      useBuildSpy = jest.mocked(useBuild);

      render(
        <FakeContextProvider>
          <BuildTable>
            <BuildTableHead>
              <GerritChangesHeadCell />
            </BuildTableHead>
            <BuildTableBody>
              <BuildTableRow build={buildWithMultipleCls}>
                <GerritChangesContentCell />
              </BuildTableRow>
            </BuildTableBody>
          </BuildTable>
        </FakeContextProvider>,
      );
    });

    afterEach(() => {
      useBuildSpy.mockClear();
      cleanup();
    });

    it('should expand/collapse correctly', async () => {
      const toggleRowButton = screen.getByLabelText('toggle-row');
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');

      expect(toggleRowButton).not.toBeDisabled();
      expect(toggleRowButton).not.toHaveStyleRule('visibility', 'hidden');

      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']"),
      ).not.toHaveStyle({ display: 'none' });
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']"),
      ).toHaveStyle({ display: 'none' });

      // Expand by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']"),
      ).toHaveStyle({ display: 'none' });
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']"),
      ).not.toHaveStyle({ display: 'none' });

      // Collapse by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']"),
      ).not.toHaveStyle({ display: 'none' });
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']"),
      ).toHaveStyle({ display: 'none' });

      // Expand again by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']"),
      ).toHaveStyle({ display: 'none' });
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']"),
      ).not.toHaveStyle({ display: 'none' });

      // Collapse by clicking on toggle button.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(
        toggleRowButton.querySelector("[data-testid='ChevronRightIcon']"),
      ).not.toHaveStyle({ display: 'none' });
      expect(
        toggleRowButton.querySelector("[data-testid='ExpandMoreIcon']"),
      ).toHaveStyle({ display: 'none' });
    });

    it('should avoid unnecessary rerendering', async () => {
      const toggleRowButton = screen.getByLabelText('toggle-row');
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Expand by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Expand by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Collapse by clicking on toggle button.
      act(() => toggleRowButton.click());
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Collapse by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      expect(useBuildSpy).toHaveBeenCalledTimes(1);
    });
  });

  describe('when there are no CLs', () => {
    let useBuildSpy: jest.MockedFunctionDeep<typeof useBuild>;
    beforeEach(() => {
      useBuildSpy = jest.mocked(useBuild);

      render(
        <FakeContextProvider>
          <BuildTable>
            <BuildTableHead>
              <GerritChangesHeadCell />
            </BuildTableHead>
            <BuildTableBody>
              <BuildTableRow build={buildWithNoCls}>
                <GerritChangesContentCell />
              </BuildTableRow>
            </BuildTableBody>
          </BuildTable>
        </FakeContextProvider>,
      );
    });

    afterEach(() => {
      useBuildSpy.mockClear();
      cleanup();
    });

    it('should disable toggle button', async () => {
      const toggleButton = screen.getByLabelText('toggle-row');

      expect(toggleButton).toBeDisabled();
      expect(toggleButton).toHaveStyleRule('visibility', 'hidden');
    });

    it('should avoid unnecessary rerendering', async () => {
      const toggleAllRowsButton = screen.getByLabelText('toggle-all-rows');
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Expand by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There are no CLs anyway.
      expect(useBuildSpy).toHaveBeenCalledTimes(1);

      // Collapse by changing the default state.
      act(() => toggleAllRowsButton.click());
      await act(() => jest.runAllTimersAsync());
      // Not rerendered. There are no CLs anyway.
      expect(useBuildSpy).toHaveBeenCalledTimes(1);
    });
  });
});
