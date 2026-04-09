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

import {
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Box,
  Checkbox,
  TextField,
} from '@mui/material';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { StringListFilterCategoryBuilder } from '../filters/string_list_filter';
import { FilterCategory } from '../filters/use_filters';

import { FilterBar } from './filter_bar';

// Mock useDeferredValue to avoid delays in tests
jest.mock('react', () => ({
  ...jest.requireActual('react'),
  useDeferredValue: <T,>(val: T) => val,
}));

// Mock @tanstack/react-virtual as JSDOM doesn't support layout/measurement
jest.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }).map((_, i) => ({
        index: i,
        start: i * 20,
        size: 20,
        measureElement: () => {},
      })),
    getTotalSize: () => count * 20,
    scrollToIndex: () => {},
  }),
}));

const TestComponent = ({
  renderDuplicate = false,
  initialOptions = [
    { label: 'Option 1', key: 'option-1', options: ['Value 1'] },
    { label: 'Option 2', key: 'option-2', options: ['Value 1'] },
  ],
}: {
  renderDuplicate?: boolean;
  initialOptions?: { label: string; key: string; options: string[] }[];
}) => {
  const [filterCategories, setFilterCategories] = useState<FilterCategory[]>(
    () => {
      const initCategories: FilterCategory[] = [];
      initialOptions.forEach((o) => {
        const builder = new StringListFilterCategoryBuilder()
          .setLabel(o.label)
          .setOptions(o.options.map((val) => ({ label: val, value: val })));

        const category = builder.build(
          o.key,
          (newFilter) => {
            setFilterCategories((prev) =>
              prev.map((f) => (f.key === newFilter.key ? newFilter : f)),
            );
          },
          undefined, // terms
        );
        initCategories.push(category);
      });
      return initCategories;
    },
  );

  return (
    <ShortcutProvider>
      <FilterBar
        filterCategoryDatas={filterCategories}
        onApply={() => {}}
        data-testid="filter-bar"
      />
      {renderDuplicate && (
        <FilterBar
          filterCategoryDatas={filterCategories}
          onApply={() => {}}
          data-testid="filter-bar-2"
        />
      )}
      <TextField data-testid="text-field"></TextField>
      <Checkbox data-testid="checkbox"></Checkbox>
      <TextField data-testid="disabled-field" disabled label="Disabled Input" />

      <FormControl fullWidth data-testid="dropdown-container">
        <InputLabel id="dropdown-label">Age</InputLabel>
        <Select
          labelId="dropdown-label"
          id="demo-simple-select"
          label="Age"
          defaultValue=""
        >
          <MenuItem value={10}>Ten</MenuItem>
        </Select>
      </FormControl>
    </ShortcutProvider>
  );
};

const getSearchBarInput = () => {
  const searchInput = screen.getByTestId('search-bar');
  return within(searchInput).getByRole('textbox');
};

const getCheckbox = () => {
  const container = screen.getByTestId('checkbox');
  return within(container).getByRole('checkbox');
};

const getTextField = () => {
  const container = screen.getByTestId('text-field');
  return within(container).getByRole('textbox');
};

const getDropdown = () => {
  const container = screen.getByTestId('dropdown-container');
  return within(container).getByRole('combobox');
};

const getDisabledInput = () => {
  const container = screen.getByTestId('disabled-field');
  return within(container).getByRole('textbox');
};

// used on our fuzzy search matches, which break up the text into separate spans
const queryByBrokenUpText = (text: string) => {
  return screen.queryAllByText((_, element) => {
    // Use `element.textContent` to get the full, combined text
    return element?.textContent === text;
  })[0];
};

describe('FilterBar', () => {
  it('should render and open dropdown on focus', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    const user = userEvent.setup();

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();

    const searchInput = getSearchBarInput();

    await user.click(searchInput);

    expect(screen.getByText('Option 1')).toBeInTheDocument();
    expect(screen.getByText('Option 2')).toBeInTheDocument();
  });

  it('should allow selecting and applying a filter', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    expect(screen.getByText('1 | [ Option 1 ]: Value 1')).toBeInTheDocument();
  });

  it('should allow moving between chips with arrows', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    await user.click(searchInput);

    await user.click(screen.getByText('Option 2'));
    await user.click(screen.getAllByText('Value 1')[0]);
    await user.click(screen.getByText('Apply'));

    const chip1 = screen.getByText('1 | [ Option 1 ]: Value 1');
    const chip2 = screen.getByText('1 | [ Option 2 ]: Value 1');

    expect(chip1).toBeInTheDocument();
    expect(chip2).toBeInTheDocument();

    await user.click(searchInput);

    await user.keyboard('{Home}');
    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toContainElement(chip2);

    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toContainElement(chip1);

    await user.keyboard('{Delete}');

    expect(chip1).not.toBeInTheDocument();
  });

  it('should not move focus to chips with arrows if cursor is not at the beginning', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    await user.click(searchInput);
    await user.type(searchInput, 'abc');

    // Press arrow left (cursor moves to between b and c)
    await user.keyboard('{ArrowLeft}');

    // Focus should STILL be on the search input
    expect(document.activeElement).toBe(searchInput);
  });

  it('should allow closing the dropdown on escape', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    const option1 = screen.getByText('Option 1');
    expect(option1).toBeInTheDocument();

    await user.keyboard('{Escape}');

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
  });

  it('should open dropdown again after typing', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.keyboard('Opt');
    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();

    await user.keyboard('{Escape}');

    expect(queryByBrokenUpText('Option 1')).toBeUndefined();

    await user.keyboard('i');
    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
  });

  it('should open dropdown again after clicking on searchbar', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.keyboard('Opt');
    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();

    await user.keyboard('{Escape}');

    expect(queryByBrokenUpText('Option 1')).toBeUndefined();

    await user.click(searchInput);
    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
  });

  it('should display empty state when no results are found', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);
    await user.type(searchInput, 'AAAAAAAAAA');

    expect(screen.getByText('No results')).toBeInTheDocument();
  });

  it('should display filtered results', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);
    await user.type(searchInput, 'Option 1');

    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
    expect(queryByBrokenUpText('Option 2')).toBeUndefined();
  });

  it('should append to the search input when typing within the dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.keyboard('Opt');

    await user.keyboard('{ArrowDown}');

    await user.keyboard('i');

    expect(searchInput).toHaveValue('Opti');
  });

  it('should autocomplete category name when typing within secondary menu', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    const option1 = screen.getByText('Option 1');
    await user.click(option1);

    // Wait for the secondary menu to render and focus
    const secondaryMenuButton = await screen.findByRole('checkbox', {
      name: /value 1/i,
    });
    expect(secondaryMenuButton).toBeInTheDocument();

    // Secondary menu is open. Now type.
    await user.keyboard('a');

    // The search input should be updated with autocomplete.
    // Label should be autocompleted, not value
    await waitFor(() => {
      expect(searchInput).toHaveValue('Option 1:a');
    });
  });

  it.skip('should close secondary menu when primary menu item has been filtered out', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    await user.keyboard('Opt');

    await user.keyboard('{ArrowDown}');

    await user.keyboard('{Enter}');

    // Wait for secondary menu to open
    const secondaryMenuButton = await screen.findByRole('checkbox', {
      name: /value 1/i,
    });
    expect(secondaryMenuButton).toBeInTheDocument();

    // Select all text and replace with new text
    // Focus search bar first to type in it
    searchInput.focus();
    await user.keyboard('{Backspace}{Backspace}{Backspace}'); // can't get ctrl + a work in tests
    await user.keyboard('zzzz');

    await waitFor(() => {
      expect(searchInput).toHaveValue('zzzz');
    });
    await waitFor(() => {
      expect(screen.queryByText('Value 1')).not.toBeInTheDocument();
    });
    expect(queryByBrokenUpText('Option 1')).toBeUndefined();
  });

  it('should allow deleting chips with backspace', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    const chip = screen.getByText('1 | [ Option 1 ]: Value 1');
    expect(chip).toBeInTheDocument();

    // Focus input and press backspace when cursor is at the beginning.
    searchInput.focus();
    await user.keyboard('{Backspace}');

    expect(chip).not.toBeInTheDocument();
  });

  it('reopening a closed dropdown should open it without secondary menu', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.keyboard('{ArrowDown}');
    await user.keyboard('{ArrowRight}');

    // arrow up is dependant on the OptionComponent passed, so we have to manually click on the search input
    await user.click(searchInput);

    expect(screen.queryByText('Value 1')).toBeInTheDocument();
    expect(searchInput).toHaveFocus();

    // close dropdown
    await user.keyboard('{Escape}');

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Value 1')).not.toBeInTheDocument();

    await user.keyboard('v');

    // after dropdown was closed when we reopen it secondary menu should be closed
    expect(screen.queryByText('Option 1')).toBeInTheDocument();
    expect(screen.queryByText('Value 1')).not.toBeInTheDocument();
  });

  it('should delete a focused chip with backspace', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    const chip1 = screen.getByText('1 | [ Option 1 ]: Value 1');
    expect(chip1).toBeInTheDocument();

    // Add a filter to get a chip.
    await user.click(screen.getByText('Option 2'));
    await user.click(screen.getAllByText('Value 1')[0]);
    await user.click(screen.getByText('Apply'));

    const chip2 = screen.getByText('1 | [ Option 2 ]: Value 1');
    expect(chip2).toBeInTheDocument();

    // Focus input and press backspace when cursor is at the beginning.
    searchInput.focus();

    await user.keyboard('{ArrowLeft}');
    await user.keyboard('{ArrowLeft}');
    await user.keyboard('{Backspace}');

    expect(chip1).not.toBeInTheDocument();
    expect(chip2).toBeInTheDocument();
  });

  it('should allow editing the chips value with a mouse', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    // Add a filter.
    const searchInput = getSearchBarInput();

    await user.click(searchInput);
    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    const chip = screen.getByText('1 | [ Option 1 ]: Value 1');

    // Click the chip to open its dropdown.
    await user.click(chip);

    // The dropdown for the chip contains the TestOptionComponent.
    const optionButton = screen.getByRole('checkbox', { name: /value 1/i });
    expect(optionButton).toBeInTheDocument();
    await user.click(optionButton);

    const applyButton = screen.getByRole('button', { name: /apply/i });
    await user.click(applyButton);

    // We unchecked the option, the chip goes away.
    expect(
      screen.queryByText('1 | [ Option 1 ]: Value 1'),
    ).not.toBeInTheDocument();
  });

  it('should allow focusing on search bar with keyboard from dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    const option1 = screen.getByText('Option 1');
    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(option1);

    await user.keyboard('{ArrowUp}');

    expect(searchInput).toHaveFocus();

    await user.keyboard('{Control>}j{/Control}');

    expect(document.activeElement).toContainElement(option1);

    await user.keyboard('{Control>}k{/Control}');

    expect(searchInput).toHaveFocus();
  });

  it('should focus on secondary menu with arrow down if secondary menu is open', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    // Open a category to have a secondary menu.
    const option1 = screen.getByText('Option 1');
    await user.click(option1);

    const secondaryMenuButton = screen.getByRole('checkbox', {
      name: /value 1/i,
    });

    expect(secondaryMenuButton).toBeInTheDocument();

    // Focus the search bar again.
    await user.click(searchInput);
    // Press arrow down.
    await user.keyboard('{ArrowDown}');

    await waitFor(() => {
      expect(document.activeElement).toContainElement(secondaryMenuButton);
    });
  });

  it('should open and close secondary menu with arrows left and right', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();
    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    const option1 = screen.getByText('Option 1');
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toContainElement(option1);

    await user.keyboard('{ArrowRight}');
    await waitFor(() => {
      const secondaryMenuButton = screen.getByRole('checkbox', {
        name: /value 1/i,
      });
      expect(secondaryMenuButton).toBeInTheDocument();
      expect(document.activeElement).toContainElement(secondaryMenuButton);
    });

    await user.keyboard('{ArrowLeft}');
    expect(
      screen.queryByRole('checkbox', { name: /value 1/i }),
    ).not.toBeInTheDocument();
    expect(document.activeElement).toContainElement(option1);

    // test going back and forth with vim navigation
    await user.keyboard('{Control>}l{/Control}');

    await waitFor(() => {
      const secondaryMenuButton = screen.getByRole('checkbox', {
        name: /value 1/i,
      });

      expect(secondaryMenuButton).toBeInTheDocument();
      expect(document.activeElement).toContainElement(secondaryMenuButton);
    });

    await user.keyboard('{Control>}h{/Control}');
    expect(
      screen.queryByRole('checkbox', { name: /value 1/i }),
    ).not.toBeInTheDocument();
    expect(document.activeElement).toContainElement(option1);
  });

  it('should delete chips with middle mouse button', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    const user = userEvent.setup();

    const searchInput = getSearchBarInput();

    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.click(screen.getByText('Option 1'));
    await user.click(screen.getByText('Value 1'));
    await user.click(screen.getByText('Apply'));

    const chip = screen.getByText('1 | [ Option 1 ]: Value 1');
    expect(chip).toBeInTheDocument();

    // Middle click on the chip.
    fireEvent.mouseDown(chip, { button: 1 });

    expect(chip).not.toBeInTheDocument();
  });

  it('should close dropdown on clicking outside', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    expect(screen.getByText('Option 1')).toBeInTheDocument();

    await user.click(document.body);

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
  });

  it('should clear search query after applying a filter', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.type(searchInput, 'Option');
    expect(searchInput).toHaveValue('Option');

    await user.click(queryByBrokenUpText('Option 1'));

    const checkbox = await screen.findByRole('checkbox', { name: /value 1/i });
    await user.click(checkbox);
    await user.click(screen.getByText('Apply'));

    // Wait for chip to appear to ensure apply is done.
    screen.getByText('1 | [ Option 1 ]: Value 1');

    expect(searchInput).toHaveValue('');
    expect(searchInput).toHaveFocus();
  });

  it('should pass empty children search query when only category is searched', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.type(searchInput, 'Option 1:');

    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
    expect(queryByBrokenUpText('Option 2')).toBeUndefined();

    await user.click(queryByBrokenUpText('Option 1'));

    // The secondary menu should show Value 1
    const checkbox = await screen.findByRole('checkbox', { name: /value 1/i });
    expect(checkbox).toBeInTheDocument();
  });

  it('should pass correct children search query when category and value are searched', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.type(searchInput, 'Opt:Val');

    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
    expect(queryByBrokenUpText('Option 2')).toBeInTheDocument();

    await user.click(queryByBrokenUpText('Option 1'));

    // The secondary menu should show Value 1 because 'Val' matches 'Value 1'
    const checkbox = await screen.findByRole('checkbox', { name: /value 1/i });
    expect(checkbox).toBeInTheDocument();

    await user.type(searchInput, 'xyz');

    // The secondary menu shows all options greyed out instead of hiding them
    await waitFor(() => {
      expect(
        screen.getByRole('checkbox', { name: /value 1/i }),
      ).toBeInTheDocument();
    });
  });

  it('should order filter results correctly', async () => {
    render(
      <FakeContextProvider>
        <TestComponent
          initialOptions={[
            {
              label: 'Bidding',
              key: 'bidding',
              options: ['Some bidding value'], // id in the middle
            },
            {
              label: 'Test category',
              key: 'test-category',
              options: ['test-category-id'], // id at the end but part of a value
            },
            {
              label: 'Dut ID', // id at the end
              key: 'dut-id',
              options: ['Some Dut ID value'],
            },
            {
              label: 'ID', // exact match
              key: 'id',
              options: ['Some ID value'],
            },
            {
              label: 'aaiaaadaaa', // non consecutive match
              key: 'aaiaaadaaa',
              options: ['Some aaiaaadaaa value'],
            },
            {
              label: 'Non matching label',
              key: 'non-matching-label',
              options: ['Non matching value'],
            },
          ]}
        />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.type(searchInput, 'id');

    expect(queryByBrokenUpText('Dut ID')).toBeInTheDocument();
    expect(queryByBrokenUpText('ID')).toBeInTheDocument();
    expect(queryByBrokenUpText('Bidding')).toBeInTheDocument();
    expect(queryByBrokenUpText('Test category')).toBeInTheDocument();
    expect(queryByBrokenUpText('aaiaaadaaa')).toBeInTheDocument();
    expect(queryByBrokenUpText('Non matching label')).toBeUndefined();

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toHaveTextContent('ID');

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toHaveTextContent('Dut ID');

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toHaveTextContent('Test category');

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toHaveTextContent('Bidding');

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toHaveTextContent('aaiaaadaaa');
  });

  it('should allow navigation to and from selected chip dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.click(screen.getByText('Option 1'));

    // Wait for secondary menu
    const value1Checkbox = await screen.findByRole('checkbox', {
      name: /value 1/i,
    });
    await user.click(value1Checkbox);

    await user.click(screen.getByText('Apply'));

    const chip = screen.getByText('1 | [ Option 1 ]: Value 1');
    expect(chip).toBeInTheDocument();

    expect(searchInput).toHaveFocus();

    // Click the chip to open its dropdown.
    await user.click(chip);

    expect(document.activeElement).not.toContainElement(searchInput);

    // Type in chip's search input
    const chipSearchInput = screen.getByPlaceholderText(/search/i);
    await user.type(chipSearchInput, 'test_search');

    expect(chipSearchInput).toHaveValue('test_search');

    // close dropdown with Escape and focus search input
    await user.keyboard('{Escape}');
    searchInput.focus();

    expect(searchInput).toHaveFocus();
    await waitFor(() => {
      expect(
        screen.queryByRole('checkbox', { name: /value 1/i }),
      ).not.toBeInTheDocument();
    });
  });

  it('should focus on filter bar after clicking slash keyboard button without adding slash character and register the word', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    expect(searchInput).not.toHaveFocus();

    await user.keyboard('/');
    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue('');

    await user.keyboard('TEST');

    expect(searchInput).toHaveValue('TEST');
  });

  it('should focus on filter bar after clicking', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();

    await user.click(searchInput);
    expect(searchInput).toHaveFocus();

    await user.keyboard('/');
    expect(searchInput).toHaveFocus();

    expect(searchInput).toHaveValue('/');
  });

  it('should keep focus and write slash when focused in a text field', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    const textField = getTextField();

    await user.click(textField);
    textField.focus();
    expect(textField).toHaveFocus();

    await user.keyboard('/');

    expect(searchInput).not.toHaveFocus();
    expect(textField).toHaveValue('/');
  });

  it('should focus on filter bar after having focus on checkbox', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    const checkbox = getCheckbox();

    await user.click(checkbox);
    expect(checkbox).toHaveFocus();

    await user.keyboard('/');

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue('');
  });

  it('should focus on filter bar after having focus on dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    const dropdown = getDropdown();
    dropdown.focus();

    expect(dropdown).toHaveFocus();

    await user.keyboard('/');

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue('');
  });

  it('should focus on filter bar after having focus on modal', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
        <Box role="dialog" data-testid="test-modal">
          <button data-testid="modal-button">Modal Action</button>
        </Box>
      </FakeContextProvider>,
    );

    const user = userEvent.setup();
    const searchInput = getSearchBarInput();
    const modalButton = screen.getByTestId('modal-button');

    expect(searchInput).not.toHaveFocus();
    expect(modalButton).not.toHaveFocus();

    await user.click(modalButton);
    expect(modalButton).toHaveFocus();

    await user.keyboard('/');

    expect(searchInput).not.toHaveFocus();
    expect(modalButton).toHaveFocus();
  });

  it('should still have focus on rich text after pressing slash', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
        <Box data-testid="rich-text-editor">
          <div contentEditable role="textbox" />
        </Box>
      </FakeContextProvider>,
    );

    const user = userEvent.setup();
    const searchInput = getSearchBarInput();
    const richTextEditor = within(
      screen.getByTestId('rich-text-editor'),
    ).getByRole('textbox');

    expect(searchInput).not.toHaveFocus();

    await user.click(richTextEditor);
    expect(richTextEditor).toHaveFocus();
    expect(richTextEditor).toHaveTextContent('');

    await user.keyboard('/');

    expect(richTextEditor).toHaveFocus();
    expect(richTextEditor).toHaveTextContent('/');
  });

  it('should focus on filter bar after having focus on disabled input', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBarInput();
    const disInput = getDisabledInput();

    await user.click(disInput);
    await user.keyboard('/');

    expect(searchInput).toHaveFocus();
    expect(searchInput).toHaveValue('');
  });

  it('should throw a specific conflict error when duplicate shortcuts are detected', async () => {
    const consoleSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {});

    render(
      <FakeContextProvider>
        <TestComponent renderDuplicate={true} />
      </FakeContextProvider>,
    );

    const errorMessages = await screen.findAllByText(
      /Shortcut conflict detected/i,
    );
    expect(errorMessages.length).toBeGreaterThan(0);

    expect(errorMessages[0]).toHaveTextContent(/Exact conflict/);
    expect(errorMessages[0]).toHaveTextContent(/Focus search bar/);

    consoleSpy.mockRestore();
  });
});
