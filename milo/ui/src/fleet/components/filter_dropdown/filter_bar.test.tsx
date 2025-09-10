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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';

import {
  FilterCategoryData,
  OptionComponent,
  OptionComponentHandle,
  OptionComponentProps,
} from '@/fleet/components/filter_dropdown/filter_dropdown';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FilterBar } from './filter_bar';

interface TestOptionComponentProps {
  value: string;
  onAddFilter: () => void;
}

const TestOptionComponent = forwardRef<
  OptionComponentHandle,
  OptionComponentProps<TestOptionComponentProps>
>(function TestOptionComponent(
  {
    childrenSearchQuery: searchQuery,
    onSearchBarFocus,
    optionComponentProps: { value, onAddFilter },
  },
  _ref,
) {
  const buttonRef = useRef<HTMLButtonElement>(null);

  useImperativeHandle(_ref, () => ({
    focus: () => {
      buttonRef.current?.focus();
    },
  }));

  return (
    <>
      <span data-testid="test-option-component-search-query">
        {searchQuery}
      </span>
      <button
        data-testid="test-option-component-button"
        ref={buttonRef}
        onClick={() => {
          onAddFilter();
          if (onSearchBarFocus) onSearchBarFocus();
        }}
        // eslint-disable-next-line jsx-a11y/no-autofocus
        autoFocus
      >
        {value}
      </button>
    </>
  );
});

const TEST_FILTER_OPTIONS = [
  {
    label: 'Option 1',
    value: 'option-1',
    options: ['Value 1'],
  },
  {
    label: 'Option 2',
    value: 'option-2',
    options: ['Value 1'],
  },
];

const TestComponent = ({
  options,
}: {
  options: typeof TEST_FILTER_OPTIONS;
}) => {
  const [selectedOptions, setSelectedOptions] = useState<
    Record<string, readonly string[]>
  >({});

  const [tempSelectedOption, setTempSelectedOption] = useState<
    Record<string, readonly string[]>
  >({});

  useEffect(() => {
    setTempSelectedOption(selectedOptions);
  }, [selectedOptions]);

  const getChipLabel = (
    option: FilterCategoryData<TestOptionComponentProps>,
  ): string => option.label + ': ' + selectedOptions[option.value]?.join(', ');

  const selectedOptionKeys = Object.keys(selectedOptions).filter(
    (key) => (selectedOptions[key] || []).length > 0,
  );

  const filterCategoryDatas: FilterCategoryData<TestOptionComponentProps>[] =
    options.map((o) => ({
      label: o.label,
      value: o.value,
      getChildrenSearchScore: (childrenSearchQuery: string) => {
        const sortedElements = fuzzySort(childrenSearchQuery)(o.options);
        return sortedElements[0].score;
      },
      optionsComponent:
        TestOptionComponent as OptionComponent<TestOptionComponentProps>,
      optionsComponentProps: {
        value: o.options[0],
        onAddFilter: () => {
          setTempSelectedOption((prev) => ({
            ...prev,
            [o.value]: [o.options[0]],
          }));
        },
      },
    }));

  return (
    <FilterBar
      filterCategoryDatas={filterCategoryDatas}
      selectedOptions={selectedOptionKeys}
      onApply={() => {
        setSelectedOptions(tempSelectedOption);
      }}
      getChipLabel={getChipLabel}
      onChipDeleted={(option) => {
        setSelectedOptions((prev) => ({ ...prev, [option.value]: [] }));
      }}
    />
  );
};

const getSearchBar = () => {
  const searchInput = screen.getByTestId('search-bar');
  return within(searchInput).getByRole('textbox');
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
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();

    const searchInput = getSearchBar();

    fireEvent.focus(searchInput);

    await waitFor(() => {
      expect(screen.getByText('Option 1')).toBeInTheDocument();
      expect(screen.getByText('Option 2')).toBeInTheDocument();
    });
  });

  it('should allow selecting and applying a filter', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );

    const searchInput = getSearchBar();
    fireEvent.focus(searchInput);

    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });

    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });

    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    expect(await screen.findByText('Option 1: Value 1')).toBeInTheDocument();
  });

  it('should allow moving between chips with arrows', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );

    const user = userEvent.setup();

    const searchInput = getSearchBar();
    fireEvent.focus(searchInput);

    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });

    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });

    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    fireEvent.focus(searchInput);

    await act(async () => {
      fireEvent.click(await screen.findByText('Option 2'));
    });

    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });

    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip1 = await screen.findByText('Option 1: Value 1');
    const chip2 = await screen.findByText('Option 2: Value 1');

    expect(chip1).toBeInTheDocument();
    expect(chip2).toBeInTheDocument();

    fireEvent.click(searchInput);

    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toContainElement(chip2);

    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toContainElement(chip1);

    await user.keyboard('{Delete}');

    expect(chip1).not.toBeInTheDocument();
  });

  it('should allow closing the dropdown on escape', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    const option1 = await screen.findByText('Option 1');
    expect(option1).toBeInTheDocument();

    await user.keyboard('{Escape}');

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
  });

  it('should open dropdown again after typing', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
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
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
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
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);
    await user.type(searchInput, 'AAAAAAAAAA');

    // The default TestComponent's getSearchScore returns 0, which causes
    // items to be filtered out when a search query is present.
    await waitFor(() => {
      expect(screen.getByText('No results')).toBeInTheDocument();
    });
  });

  it('should display filtered results', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);
    await user.type(searchInput, 'Option 1');

    await waitFor(() => {
      expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
      expect(queryByBrokenUpText('Option 2')).toBeUndefined();
    });
  });

  it('should append to the search input when typing within the dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    await user.keyboard('Opt');

    await user.keyboard('{ArrowDown}');

    await user.keyboard('i');

    expect(searchInput).toHaveValue('Opti');
  });

  it('should autocomplete category name when typing within secondary menu', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    const option1 = await screen.findByText('Option 1');
    await user.click(option1);

    // Secondary menu is open. Now type.
    await user.keyboard('a');

    // The search input should be updated with autocomplete.
    // Label should be autocompleted, not value
    expect(searchInput).toHaveValue('Option 1:a');
  });

  it('should close secondary menu when primary menu item has been filtered out', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    await user.keyboard('Opt');

    await user.keyboard('{ArrowDown}');

    await user.keyboard('{Enter}');

    expect(
      screen.queryByTestId('test-option-component-button'),
    ).toBeInTheDocument();

    // Select all text and replace with new text
    await user.keyboard('{Backspace}{Backspace}{Backspace}'); // can't get ctrl + a work in tests
    await user.keyboard('zzzz');

    expect(searchInput).toHaveValue('zzzz');
    expect(
      screen.queryByTestId('test-option-component-button'),
    ).not.toBeInTheDocument();
    expect(queryByBrokenUpText('Option 1')).toBeUndefined();
  });

  it('should allow deleting chips with backspace', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip = await screen.findByText('Option 1: Value 1');
    expect(chip).toBeInTheDocument();

    // Focus input and press backspace when cursor is at the beginning.
    searchInput.focus();
    await user.keyboard('{Backspace}');

    expect(chip).not.toBeInTheDocument();
  });

  // b/444239211
  it('reopening a closed dropdown should open it without secondary menu', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await user.keyboard('{ArrowDown}');
    await user.keyboard('{ArrowRight}');

    // arrow up is dependant on the OptionComponent passed, so we have to manually click on the search input
    await user.click(searchInput);

    expect(await screen.queryByText('Value 1')).toBeInTheDocument();
    expect(searchInput).toHaveFocus();

    // close dropdown
    await user.keyboard('{Escape}');

    expect(await screen.queryByText('Option 1')).not.toBeInTheDocument();
    expect(await screen.queryByText('Value 1')).not.toBeInTheDocument();

    await user.keyboard('v');

    // after dropdown was closed when we reopen it secondary menu should be closed
    expect(await screen.queryByText('Option 1')).toBeInTheDocument();
    expect(await screen.queryByText('Value 1')).not.toBeInTheDocument();
  });

  // b/443967368 - backspace could be handled in 2 different ways, make sure they are handled properly
  it('should delete a focused chip with backspace', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    // Add a filter to get a chip.
    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip1 = await screen.findByText('Option 1: Value 1');
    expect(chip1).toBeInTheDocument();

    // Add a filter to get a chip.
    await act(async () => {
      fireEvent.click(await screen.findByText('Option 2'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip2 = await screen.findByText('Option 2: Value 1');
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
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    // Add a filter.

    const searchInput = getSearchBar();
    act(() => {
      fireEvent.focus(searchInput);
    });
    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip = await screen.findByText('Option 1: Value 1');

    // Click the chip to open its dropdown.
    await user.click(chip);

    // The dropdown for the chip contains the TestOptionComponent.
    const optionButton = await screen.findByTestId(
      'test-option-component-button',
    );
    expect(optionButton).toBeInTheDocument();
    await user.click(optionButton);

    // The dropdown for the chip also has an "Apply" button.
    const applyButton = screen.getByRole('button', { name: /apply/i });
    await user.click(applyButton);

    // The chip is still there. The test component doesn't support
    // changing the value, but we've tested the UI flow.
    expect(await screen.findByText('Option 1: Value 1')).toBeInTheDocument();
  });

  it('should allow focusing on search bar with keyboard from dropdown', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    const option1 = await screen.findByText('Option 1');
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
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    // Open a category to have a secondary menu.
    const option1 = await screen.findByText('Option 1');
    await user.click(option1);

    const secondaryMenuButton = await screen.findByTestId(
      'test-option-component-button',
    );

    expect(secondaryMenuButton).toBeInTheDocument();

    // Focus the search bar again.
    await user.click(searchInput);
    // Press arrow down.
    await user.keyboard('{ArrowDown}');

    expect(secondaryMenuButton).toHaveFocus();
  });

  it('should open and close secondary menu with arrows left and right', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();
    const searchInput = getSearchBar();
    await user.click(searchInput);

    const option1 = await screen.findByText('Option 1');
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toContainElement(option1);

    await user.keyboard('{ArrowRight}');
    const secondaryMenuButton = await screen.findByTestId(
      'test-option-component-button',
    );
    expect(secondaryMenuButton).toBeInTheDocument();
    expect(secondaryMenuButton).toHaveFocus();

    await user.keyboard('{ArrowLeft}');
    expect(
      screen.queryByTestId('test-option-component-button'),
    ).not.toBeInTheDocument();
    expect(document.activeElement).toContainElement(option1);
  });

  it('should delete chips with middle mouse button', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );

    const searchInput = getSearchBar();
    act(() => {
      fireEvent.focus(searchInput);
    });

    // Add a filter to get a chip.
    await act(async () => {
      fireEvent.click(await screen.findByText('Option 1'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    const chip = await screen.findByText('Option 1: Value 1');
    expect(chip).toBeInTheDocument();

    // Middle click on the chip.
    await act(async () => {
      fireEvent.mouseDown(chip, { button: 1 });
    });

    await waitFor(() => expect(chip).not.toBeInTheDocument());
  });

  it('should close dropdown on clicking outside', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.click(searchInput);

    expect(await screen.findByText('Option 1')).toBeInTheDocument();

    const backdrop = screen.getByTestId('filter-dropdown-backdrop');

    await user.click(backdrop);

    await waitFor(() => {
      expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    });
  });

  it('should clear search query after applying a filter', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.type(searchInput, 'Option');
    expect(searchInput).toHaveValue('Option');

    await act(async () => {
      fireEvent.click(queryByBrokenUpText('Option 1'));
    });
    await act(async () => {
      fireEvent.click(await screen.getByTestId('test-option-component-button'));
    });
    await act(async () => {
      fireEvent.click(screen.getByText('Apply'));
    });

    // Wait for chip to appear to ensure apply is done.
    await screen.findByText('Option 1: Value 1');

    expect(searchInput).toHaveValue('');
    expect(searchInput).toHaveFocus();
  });

  it('should pass empty children search query when only category is searched', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.type(searchInput, 'Option 1:');

    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
    expect(queryByBrokenUpText('Option 2')).toBeUndefined();

    await act(async () => {
      fireEvent.click(queryByBrokenUpText('Option 1'));
    });

    expect(
      screen.getByTestId('test-option-component-search-query'),
    ).toBeEmptyDOMElement(); // search query is empty
  });

  it('should pass correct children search query when category and value are searched', async () => {
    render(
      <FakeContextProvider>
        <TestComponent options={TEST_FILTER_OPTIONS} />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.type(searchInput, 'Opt:Val');

    expect(queryByBrokenUpText('Option 1')).toBeInTheDocument();
    expect(queryByBrokenUpText('Option 2')).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(queryByBrokenUpText('Option 1'));
    });

    expect(
      screen.getByTestId('test-option-component-search-query'),
    ).toHaveTextContent('Val');
  });

  it('should order filter results correctly', async () => {
    render(
      <FakeContextProvider>
        <TestComponent
          options={[
            {
              label: 'Bidding',
              value: 'bidding',
              options: ['Some bidding value'], // id in the middle
            },
            {
              label: 'Test category',
              value: 'test-category',
              options: ['test-category-id'], // id at the end but part of a value
            },
            {
              label: 'Dut ID', // id at the end
              value: 'dut-id',
              options: ['Some Dut ID value'],
            },
            {
              label: 'ID', // exact match
              value: 'id',
              options: ['Some ID value'],
            },
            {
              label: 'aaiaaadaaa', // non consecutive match
              value: 'aaiaaadaaa',
              options: ['Some aaiaaadaaa value'],
            },
            {
              label: 'Non matching label',
              value: 'non-matching-label',
              options: ['Non matching value'],
            },
          ]}
        />
      </FakeContextProvider>,
    );
    const user = userEvent.setup();

    const searchInput = getSearchBar();
    await user.type(searchInput, 'id');

    expect(queryByBrokenUpText('Dut ID')).toBeInTheDocument();
    expect(queryByBrokenUpText('ID')).toBeInTheDocument();
    expect(queryByBrokenUpText('Bidding')).toBeInTheDocument();
    expect(queryByBrokenUpText('Test category')).toBeInTheDocument();
    expect(queryByBrokenUpText('aaiaaadaaa')).toBeInTheDocument();
    expect(queryByBrokenUpText('Non matching label')).toBeUndefined();

    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(queryByBrokenUpText('ID'));

    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(
      queryByBrokenUpText('Dut ID'),
    );

    expect(document.activeElement).not.toContainElement(
      // make sure we are not causing false positives by focusing on all the elements at once
      queryByBrokenUpText('ID'),
    );

    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(
      queryByBrokenUpText('Test category'),
    );

    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(
      queryByBrokenUpText('Bidding'),
    );

    await user.keyboard('{ArrowDown}');

    expect(document.activeElement).toContainElement(
      queryByBrokenUpText('aaiaaadaaa'),
    );
  });
});
