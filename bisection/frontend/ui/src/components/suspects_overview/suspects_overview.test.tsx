// Copyright 2022 The LUCI Authors.
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


import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';

import { createMockPrimeSuspect } from '../../testing_tools/mocks/prime_suspect_mock';
import { SuspectsOverview } from './suspects_overview';
import { PrimeSuspect } from '../../services/luci_bisection';

describe('Test SuspectsOverview component', () => {
  test('if all suspect details are displayed', async () => {
    const mockSuspects = [
      createMockPrimeSuspect('c234de'),
      createMockPrimeSuspect('412533'),
    ];

    render(<SuspectsOverview suspects={mockSuspects} />);

    await screen.findByText('Suspect CL');

    // check there is a link for each suspect CL
    expect(screen.queryAllByRole('link')).toHaveLength(mockSuspects.length);

    // check the target URL for a suspect CL
    expect(
      screen.getByText(mockSuspects[1].cl.title).getAttribute('href')
    ).toBe(mockSuspects[1].cl.reviewURL);
  });

  test('if an appropriate message is displayed for no suspects', async () => {
    const mockSuspects: PrimeSuspect[] = [];

    render(<SuspectsOverview suspects={mockSuspects} />);

    await screen.findByText('Suspect CL');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('No suspects to display')).toBeInTheDocument();
  });
});
