// Copyright 2025 The LUCI Authors.
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

import { fireEvent, render, screen } from '@testing-library/react';

import { DutToRepair } from '../shared/types';

import { RequestRepair, generateIssueDescription } from './request_repair';

describe('<RequestRepair />', () => {
  let windowOpenSpy: jest.SpyInstance;

  beforeEach(() => {
    windowOpenSpy = jest.spyOn(window, 'open').mockImplementation(jest.fn());
  });

  afterEach(() => {
    windowOpenSpy.mockRestore();
  });

  it('should render a button that opens a new tab with the correct URL', () => {
    const selectedDuts: DutToRepair[] = [
      {
        name: 'dut1',
        dutId: 'dut1',
        state: 'needs_manual_repair',
        board: 'board1',
        model: 'model1',
        pool: 'pool1',
      },
      {
        name: 'dut2',
        dutId: 'dut2',
        state: 'needs_manual_repair',
        board: 'board2',
        model: 'model2',
        pool: 'pool2',
      },
      {
        name: 'dut3',
        dutId: 'dut3',
        state: 'ready',
        board: 'board3',
        model: 'model3',
        pool: 'pool3',
      },
      {
        name: 'dut4',
        dutId: 'dut4',
        state: 'repair_failed',
        board: 'board4',
        model: 'model4',
        pool: 'pool4',
      },
    ];

    render(<RequestRepair selectedDuts={selectedDuts} />);
    const button = screen.getByTestId('file-repair-bug-button');
    expect(button).toBeInTheDocument();

    fireEvent.click(button);

    expect(windowOpenSpy).toHaveBeenCalledTimes(1);
    const dutInfo = [
      ' * http://go/fcdut/dut1 (Location: <Please add if known>, Board: board1, Model: model1, Pool: pool1)',
      ' * http://go/fcdut/dut2 (Location: <Please add if known>, Board: board2, Model: model2, Pool: pool2)',
      ' * http://go/fcdut/dut3 (Location: <Please add if known>, Board: board3, Model: model3, Pool: pool3)',
      ' * http://go/fcdut/dut4 (Location: <Please add if known>, Board: board4, Model: model4, Pool: pool4)',
    ];
    const expectedDescription = generateIssueDescription(dutInfo.join('\n'));

    const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
    expect(openedUrl.origin).toBe('http://b');
    expect(openedUrl.pathname).toBe('/issues/new');
    expect(openedUrl.searchParams.get('markdown')).toBe('true');
    expect(openedUrl.searchParams.get('component')).toBe('575445');
    expect(openedUrl.searchParams.get('template')).toBe('1509031');
    expect(openedUrl.searchParams.get('title')).toBe(
      `[Location Unknown][Repair][board1.model1] Pool: [pool1] [dut1] and 3 more`,
    );
    expect(decodeURIComponent(openedUrl.searchParams.get('description')!)).toBe(
      decodeURIComponent(expectedDescription),
    );
    expect(windowOpenSpy).toHaveBeenCalledWith(
      expect.stringContaining(''),
      '_blank',
    );
  });

  it.each([
    {
      case: 'no DUTs are selected',
      duts: [] as DutToRepair[],
    },
  ])('should not render the button if $case', ({ duts }) => {
    render(<RequestRepair selectedDuts={duts} />);
    const button = screen.queryByTestId('file-repair-bug-button');
    expect(button).not.toBeInTheDocument();
  });
});
