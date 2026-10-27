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

import { fireEvent, render, screen } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { LabSelector } from './lab_selector';

describe('LabSelector', () => {
  beforeEach(() => {
    localStorage.clear();
    jest.clearAllMocks();
  });

  it('renders button with MTV by default and updates selection upon choosing option', () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/p/chromeos/repairs"
        routerOptions={{
          initialEntries: ['/ui/fleet/p/chromeos/repairs'],
        }}
      >
        <LabSelector />
      </FakeContextProvider>,
    );

    const button = screen.getByTestId('repair-lab-select');
    expect(button).toBeInTheDocument();
    expect(button).toHaveTextContent('Lab: MTV');

    // Click button to open popover
    fireEvent.click(button);
    expect(screen.getByText('MTV (Mountain View)')).toBeInTheDocument();
    expect(screen.getByText('EM25 (Palo Alto)')).toBeInTheDocument();
    expect(screen.getByText('IAD65 (Ashburn)')).toBeInTheDocument();

    // Select EM25
    fireEvent.click(screen.getByTestId('repair-lab-option-EM25'));
    expect(button).toHaveTextContent('Lab: EM25');
  });

  it('supports controlled value and onChange props', () => {
    const handleChange = jest.fn();
    render(
      <FakeContextProvider>
        <LabSelector value="IAD65" onChange={handleChange} />
      </FakeContextProvider>,
    );

    const button = screen.getByTestId('repair-lab-select');
    expect(button).toHaveTextContent('Lab: IAD65');

    fireEvent.click(button);
    fireEvent.click(screen.getByTestId('repair-lab-option-MTV'));
    expect(handleChange).toHaveBeenCalledWith('MTV');
  });
});
