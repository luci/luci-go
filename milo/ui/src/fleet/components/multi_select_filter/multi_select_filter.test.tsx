// Copyright 2024 The LUCI Authors.
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
  screen,
  render,
  cleanup,
  fireEvent,
  act,
  within,
} from '@testing-library/react';
import { useEffect, useState } from 'react';

import { TEST_FILTER_OPTIONS } from './mock_data';
import { SelectedFilters } from './types';

import { MultiSelectFilter } from '.';

const mockSelectedOptions = jest.fn();

const TestComponent = () => {
  const [selectedOptions, setSelectedOptions] = useState<SelectedFilters>({});
  useEffect(() => {
    mockSelectedOptions(selectedOptions);
  }, [selectedOptions]);

  return (
    <MultiSelectFilter
      filterOptions={TEST_FILTER_OPTIONS}
      selectedOptions={selectedOptions}
      setSelectedOptions={(...args) => {
        setSelectedOptions(...args);
      }}
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

describe('<MultiSelectFilter />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
    mockSelectedOptions.mockClear();
  });

  it('should be able to select options', async () => {
    render(<TestComponent />);

    click(['+ add filter', 'Option 1', 'The first option', 'Confirm']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': ['o11'],
      },
    });
    await act(() => jest.runAllTimersAsync());

    click([
      '1 | [ Option 1 ]: The first option',
      'The second option',
      'Confirm',
    ]);
    expect(
      screen.queryByText(
        '2 | [ Option 1 ]: The first option, The second option',
      ),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': ['o11', 'o12'],
      },
    });
    await act(() => jest.runAllTimersAsync());

    click(['+ add filter', 'Option 2', 'The second option', 'Confirm']);
    expect(
      screen.queryByText(
        '2 | [ Option 1 ]: The first option, The second option',
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('1 | [ Option 2 ]: The second option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': ['o11', 'o12'],
        'val-2': ['o22'],
      },
    });
  });

  it('should cancel the selection when clicking on cancel', () => {
    render(<TestComponent />);

    click(['+ add filter', 'Option 1', 'The first option', 'Cancel']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).not.toBeInTheDocument();
    expect(mockSelectedOptions).not.toHaveBeenLastCalledWith({
      'val-1': { o11: true },
    });
    expect(mockSelectedOptions).not.toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': ['o11'],
      },
    });
  });

  it('should remove a filter when clicking on the x', () => {
    render(<TestComponent />);

    click(['+ add filter', 'Option 1', 'The first option', 'Confirm']);
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': ['o11'],
      },
    });

    fireEvent.click(screen.getByTestId('CancelIcon'));
    expect(
      screen.queryByText('1 | [ Option 1 ]: The first option'),
    ).not.toBeInTheDocument();
    expect(mockSelectedOptions).toHaveBeenLastCalledWith({
      'default-namespace': {
        'val-1': [],
      },
    });
  });

  describe('should work with a keyboard', () => {
    it('should be able to select options with keyboard', async () => {
      render(<TestComponent />);

      screen.getByText('+ add filter').parentElement!.focus();
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
        'default-namespace': {
          'val-1': ['o12'],
        },
      });
    });

    it('should clear the search on backspace', () => {
      render(<TestComponent />);
      act(() => screen.getByText('+ add filter').click());

      const search = screen.getByPlaceholderText('search');

      const searchQuery = 'Opt';
      fireEvent.change(search, { target: { value: searchQuery } });
      expect(search).toHaveValue(searchQuery);

      fireEvent.keyDown(screen.getAllByRole('menuitem')[0], BACKSPACE_KEY);
      expect(search).toHaveValue('');
      expect(search).toHaveFocus();
    });

    it('should focus on search when typing', () => {
      render(<TestComponent />);
      act(() => screen.getByText('+ add filter').click());

      const search = screen.getByPlaceholderText('search');

      fireEvent.keyDown(screen.getByText('Option 1'), A_KEY);
      expect(search).toHaveValue('a');
      expect(search).toHaveFocus();
    });

    it('should go up and down with ctrl+j/k', () => {
      render(<TestComponent />);
      act(() => screen.getByText('+ add filter').click());

      screen.getByText('Option 1').focus();

      fireEvent.keyDown(document.activeElement!, CTRL_J_KEY);
      fireEvent.keyDown(document.activeElement!, CTRL_J_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 1');

      fireEvent.keyDown(document.activeElement!, CTRL_J_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 2');

      fireEvent.keyDown(document.activeElement!, CTRL_K_KEY);
      expect(document.activeElement!.nodeName.toLowerCase()).toBe('li');
      expect(document.activeElement).toContainHTML('Option 1');

      fireEvent.keyDown(document.activeElement!, CTRL_K_KEY);
      fireEvent.keyDown(document.activeElement!, SPACE_KEY);
      expect(screen.getByPlaceholderText('search')).toHaveFocus();
    });
  });
});
