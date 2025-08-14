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

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react';
import { useEffect, useState } from 'react';

import { mockVirtualizedListDomProperties } from '@/fleet/testing_tools/dom_mocks';
import { SelectedOptions } from '@/fleet/types';

import { DeviceListFilterBarOld } from './device_list_filter_bar_old';
import { TEST_FILTER_OPTIONS } from './mock_data';

const mockSelectedOptions = jest.fn();

const TestComponent = () => {
  const [selectedOptions, setSelectedOptions] = useState<SelectedOptions>({});
  useEffect(() => {
    mockSelectedOptions(selectedOptions);
  }, [selectedOptions]);

  return (
    <DeviceListFilterBarOld
      filterOptions={TEST_FILTER_OPTIONS}
      selectedOptions={selectedOptions}
      onSelectedOptionsChange={setSelectedOptions}
    />
  );
};

function click(texts: string[]) {
  texts.forEach((txt) => fireEvent.click(screen.getByText(txt)));
}
function keyDown(key: object) {
  fireEvent.keyDown(document.activeElement!, key);
}

const DOWN_KEY = {
  key: 'ArrowDown',
  code: 'ArrowDown',
};
const RIGHT_KEY = {
  key: 'ArrowRight',
  code: 'ArrowRight',
};
const SPACE_KEY = {
  key: ' ',
  code: 'Space',
};
const ENTER_KEY = {
  key: 'Enter',
  code: 'Enter',
};
const CTRL_ENTER_KEY = {
  key: 'Enter',
  code: 'Enter',
  ctrlKey: true,
};
const BACKSPACE_KEY = {
  key: 'Backspace',
  code: 'Backspace',
};
const A_KEY = {
  key: 'a',
  code: 'KeyA',
};
const CTRL_J_KEY = {
  key: 'j',
  code: 'KeyJ',
  ctrlKey: true,
};
const CTRL_K_KEY = {
  key: 'k',
  code: 'KeyK',
  ctrlKey: true,
};

describe('<DeviceListFilterBarOld />', () => {
  let cleanupDomMocks: () => void;

  beforeEach(() => {
    jest.useFakeTimers();
    cleanupDomMocks = mockVirtualizedListDomProperties();
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
    mockSelectedOptions.mockClear();
    cleanupDomMocks();
  });

  it('should be able to select options', async () => {
    render(<TestComponent />);

    click(['Add filter', 'Option 1', 'The first option', 'Apply']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'val-1': ['o11'],
    } as SelectedOptions);
    await act(() => jest.runAllTimersAsync());

    click(['1 | [ Option 1 ]: The first option', 'The second option', 'Apply']);
    expect(
      screen.queryByText(
        '2 | [ Option 1 ]: The first option, The second option',
      ),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'val-1': ['o11', 'o12'],
    });
    await act(() => jest.runAllTimersAsync());

    click(['Add filter', 'Option 2', 'The second option', 'Apply']);
    expect(
      screen.queryByText(
        '2 | [ Option 1 ]: The first option, The second option',
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('1 | [ Option 2 ]: The second option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'val-1': ['o11', 'o12'],
      'val-2': ['o22'],
    });
  });

  it('should work correctly with filtered values', async () => {
    render(<TestComponent />);

    click(['Add filter']);

    const search = screen.getByPlaceholderText('search');

    const searchQuery = 'third';
    fireEvent.change(search, { target: { value: searchQuery } });
    expect(search).toHaveValue(searchQuery);

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Option 4')).toBeInTheDocument();

    click(['Option 4']);

    const thirdOption = screen.getAllByText(
      // this search is agnostic to our highlighting mechanism
      (_, element) => element?.textContent === 'The third option',
    )[0];

    expect(thirdOption).toBeInTheDocument();

    fireEvent.click(thirdOption);
    click(['Apply']);

    expect(
      screen.queryByText('1 | [ Option 4 ]: The third option'),
    ).toBeInTheDocument();
  });

  it('should cancel the selection when clicking on cancel', () => {
    render(<TestComponent />);

    click(['Add filter', 'Option 1', 'The first option', 'Cancel']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).not.toBeInTheDocument();
    expect(mockSelectedOptions).not.toHaveBeenLastCalledWith({
      'val-1': { o11: true },
    });
    expect(mockSelectedOptions).not.toHaveBeenLastCalledWith({
      'val-1': ['o11'],
    });
  });

  it('should clear the selection when changing the open category', () => {
    render(<TestComponent />);

    click(['Add filter', 'Option 1', 'The first option']);
    expect(
      within(screen.getByText('The first option').closest('li')!).getByRole(
        'checkbox',
      ),
    ).toBeChecked();

    click(['Option 2', 'Option 1']);

    expect(
      within(screen.getByText('The first option').closest('li')!).getByRole(
        'checkbox',
      ),
    ).not.toBeChecked();
  });

  it('should remove a filter when clicking on the x', () => {
    render(<TestComponent />);

    click(['Add filter', 'Option 1', 'The first option', 'Apply']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'val-1': ['o11'],
    });

    fireEvent.click(screen.getByTestId('CancelIcon'));
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).not.toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'val-1': [],
    });
  });

  describe('should work with a keyboard', () => {
    it('should be able to select options with keyboard', async () => {
      render(<TestComponent />);

      act(() => screen.getByText('Add filter').parentElement!.focus());
      keyDown(ENTER_KEY);
      expect(document.activeElement).toContainHTML('search');

      keyDown(DOWN_KEY);
      keyDown(DOWN_KEY);
      keyDown(RIGHT_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('The first option');

      fireEvent.keyDown(document.activeElement!, SPACE_KEY);
      expect(
        within(document.activeElement! as HTMLElement).getByRole('checkbox'),
      ).toBeChecked();

      fireEvent.keyDown(document.activeElement!, SPACE_KEY);
      expect(
        within(document.activeElement! as HTMLElement).getByRole('checkbox'),
      ).not.toBeChecked();

      keyDown(DOWN_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('The second option');

      // You can use both enter and space to select options
      fireEvent.keyDown(document.activeElement!, ENTER_KEY);
      expect(
        within(document.activeElement! as HTMLElement).getByRole('checkbox'),
      ).toBeChecked();

      act(() => keyDown(CTRL_ENTER_KEY));
      expect(
        screen.queryByText('1 | [ Option 1 ]: The second option'),
      ).toBeInTheDocument();
      expect(mockSelectedOptions).toHaveBeenLastCalledWith({
        'val-1': ['o12'],
      });
    });

    it('should clear the search on backspace', async () => {
      render(<TestComponent />);
      act(() => screen.getByText('Add filter').click());

      const search = screen.getByPlaceholderText('search');

      const searchQuery = 'Opt';
      fireEvent.change(search, { target: { value: searchQuery } });
      expect(search).toHaveValue(searchQuery);

      await act(async () =>
        fireEvent.keyDown(screen.getAllByRole('menu')[0], BACKSPACE_KEY),
      );
      expect(screen.getByPlaceholderText('search')).toHaveFocus();
      expect(screen.getByPlaceholderText('search')).toHaveValue('');
    });

    it('should focus on search when typing', () => {
      render(<TestComponent />);
      act(() => screen.getByText('Add filter').click());

      const search = screen.getByPlaceholderText('search');

      fireEvent.keyDown(screen.getByText('Option 1'), A_KEY);
      expect(search).toHaveValue('a');
      expect(search).toHaveFocus();
    });

    it('should go up and down with ctrl+j/k', () => {
      render(<TestComponent />);
      act(() => screen.getByText('Add filter').click());

      act(() => fireEvent.keyDown(document.activeElement!, CTRL_J_KEY));
      act(() => fireEvent.keyDown(document.activeElement!, CTRL_J_KEY));
      // fireEvent.keyDown(document.activeElement!, CTRL_J_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 1');

      act(() => fireEvent.keyDown(document.activeElement!, CTRL_J_KEY));
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 2');

      act(() => fireEvent.keyDown(document.activeElement!, CTRL_K_KEY));
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 1');

      act(() => fireEvent.keyDown(document.activeElement!, CTRL_K_KEY));
      expect(document.activeElement!).toContainElement(
        screen.getByPlaceholderText('search'),
      );

      act(() => fireEvent.keyDown(document.activeElement!, SPACE_KEY));
      expect(screen.getByPlaceholderText('search')).toHaveFocus();
    });
  });
});
