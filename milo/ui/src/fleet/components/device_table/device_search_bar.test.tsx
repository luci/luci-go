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
// limitations under the License.n
import { fireEvent, render, screen } from '@testing-library/react';

import { OptionValue } from '@/fleet/types/option';

import { DeviceSearchBar } from './device_search_bar';

describe('<DeviceSearchBar />', () => {
  const options: OptionValue[] = [
    { value: 'test-device-2', label: 'Test Device 2' },
    { value: 'device-2', label: 'Device 2' },
    { value: 'device-1', label: 'Device 1' },
    { value: 'test-device-1', label: 'Test Device 1' },
  ];
  const applySelectedOption = jest.fn();

  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    applySelectedOption.mockClear();
  });

  it('renders correctly', async () => {
    render(
      <DeviceSearchBar
        options={options}
        applySelectedOption={applySelectedOption}
      />,
    );

    expect(screen.getByRole('textbox')).toHaveValue('');
    expect(screen.queryByText('Device 1')).not.toBeInTheDocument();
  });

  it('filters options based on search query', async () => {
    render(
      <DeviceSearchBar
        options={options}
        applySelectedOption={applySelectedOption}
        numSuggestions={2}
      />,
    );

    const searchQuery = 'device-2';
    const searchInput = screen.getByRole('textbox');

    fireEvent.keyDown(searchInput, {
      code: 'ArrowDown',
      target: { value: searchQuery },
    });
    // Value is set only after an option is selected.
    expect(searchInput).toHaveValue('');

    const device1 = screen.queryByText('Device 1');
    const device2 = screen.queryByText('Device 2');
    const testDevice1 = screen.queryByText('Test Device 1');
    const testDevice2 = screen.queryByText('Test Device 2');

    expect(device1).not.toBeInTheDocument();
    expect(device2).toBeInTheDocument();
    // We should only have one additional suggestion;
    // the next item in the sorted list of options.
    expect(testDevice1).toBeInTheDocument();
    expect(testDevice2).not.toBeInTheDocument();

    fireEvent.click(testDevice1 as HTMLElement);
    expect(searchInput).toHaveValue('Test Device 1');
    expect(applySelectedOption).toHaveBeenCalledWith('test-device-1');
  });

  it('displays a message when there are no suggestions', async () => {
    render(
      <DeviceSearchBar
        options={options}
        applySelectedOption={applySelectedOption}
      />,
    );
    fireEvent.keyDown(screen.getByRole('textbox'), {
      code: 'ArrowDown',
      target: { value: 'device-3' },
    });
    expect(screen.queryByText('No matching devices found')).toBeInTheDocument();
  });
});
