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
      { name: 'dut1', dutId: '123', state: 'needs_manual_repair' },
      { name: 'dut2', dutId: '456', state: 'needs_manual_repair' },
    ];

    render(<RequestRepair selectedDuts={selectedDuts} />);
    const button = screen.getByTestId('file-repair-bug-button');
    expect(button).toBeInTheDocument();

    fireEvent.click(button);

    expect(windowOpenSpy).toHaveBeenCalledTimes(1);
    const expectedDescription = generateIssueDescription(
      [
        ' * http://go/fcdut/dut1 (Location: <Please add if known>)',
        ' * http://go/fcdut/dut2 (Location: <Please add if known>)',
      ].join('\n'),
    );

    const finalUrl = `http://b/issues/new?component=575445&template=1509031&description=${expectedDescription}&markdown=true`;
    expect(windowOpenSpy).toHaveBeenCalledWith(finalUrl, '_blank');
  });
});
