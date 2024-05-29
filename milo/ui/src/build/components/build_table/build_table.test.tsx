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

import { render, screen } from '@testing-library/react';
import { act } from 'react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildTable } from './build_table';
import { BuildTableBody } from './build_table_body';
import { BuildTableHead } from './build_table_head';
import { SummaryHeadCell } from './summary_column';

describe('BuildTable', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('should notify default state update correctly', async () => {
    const onExpandSpy = jest.fn((_expanded: boolean) => {});
    render(
      <FakeContextProvider>
        <BuildTable
          initDefaultExpanded={true}
          onDefaultExpandedChanged={onExpandSpy}
        >
          <BuildTableHead>
            <SummaryHeadCell />
          </BuildTableHead>
          <BuildTableBody>
            <></>
          </BuildTableBody>
        </BuildTable>
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    // Don't notify on first render.
    expect(onExpandSpy).not.toHaveBeenCalled();

    const toggleButton = screen.getByLabelText('toggle-all-rows');

    // Collapse by clicking on toggle button.
    act(() => toggleButton.click());
    expect(onExpandSpy).toHaveBeenNthCalledWith(1, false);

    // Expand by clicking on toggle button.
    act(() => toggleButton.click());
    expect(onExpandSpy).toHaveBeenNthCalledWith(2, true);
  });
});
