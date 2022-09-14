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

import { RevertCL } from '../../services/luci_bisection';
import { RevertCLOverview } from './revert_cl_overview';
import { createMockRevertCL } from '../../testing_tools/mocks/revert_cl_mock';

describe('Test ChangeListOverview component', () => {
  test('if all change list details are displayed', async () => {
    const mockRevertCL = createMockRevertCL('12835');

    render(<RevertCLOverview revertCL={mockRevertCL} />);

    await screen.findByTestId('change_list_overview_table_body');

    const expectedStaticFields = [
      ['status', 'status'],
      ['submitted time', 'submitTime'],
      ['commit position', 'commitPosition'],
    ];

    // check static field labels and values are displayed
    expectedStaticFields.forEach(([label, property]) => {
      const fieldLabel = screen.getByText(new RegExp(`^(${label})$`, 'i'));
      expect(fieldLabel).toBeInTheDocument();
      expect(fieldLabel.nextSibling?.textContent).toBe(
        `${mockRevertCL[property as keyof RevertCL]}`
      );
    });

    // check the link to the code review is displayed
    expect(screen.getByText(mockRevertCL.cl.title).getAttribute('href')).toBe(
      mockRevertCL.cl.reviewURL
    );
  });
});
