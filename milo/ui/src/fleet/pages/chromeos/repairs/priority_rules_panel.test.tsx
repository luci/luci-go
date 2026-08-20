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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import * as UsePriorityRulesModule from '@/fleet/pages/chromeos/repairs/use_priority_rules';
import { ChromeOSFilterKey } from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';
import * as UseChromeOSFiltersModule from '@/fleet/pages/device_list_page/chromeos/use_chromeos_filters';
import { PriorityRule } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { PriorityRulesPanel } from './priority_rules_panel';

const MOCK_RULES: readonly PriorityRule[] = [
  {
    id: '1',
    expressionAip160: 'board = "bria"',
    weight: '100',
  },
  {
    id: '2',
    expressionAip160: 'pool = "DUT_POOL_QUOTA"',
    weight: '250',
  },
  {
    id: '3',
    expressionAip160: 'model = "volteer"',
    weight: '-500',
  },
  {
    id: '4',
    expressionAip160: 'lab = "MTV"',
    weight: '1000',
  },
];

const createMockFilterBuilders = () =>
  ({
    board: new StringListFilterCategoryBuilder().setLabel('Board').setOptions([
      { label: 'bria', value: 'bria' },
      { label: 'brya', value: 'brya' },
    ]),
    pool: new StringListFilterCategoryBuilder().setLabel('Pool').setOptions([
      { label: 'DUT_POOL_QUOTA', value: 'DUT_POOL_QUOTA' },
      { label: 'faft-cr50', value: 'faft-cr50' },
    ]),
    model: new StringListFilterCategoryBuilder().setLabel('Model').setOptions([
      { label: 'volteer', value: 'volteer' },
      { label: 'corsola', value: 'corsola' },
      { label: 'brya', value: 'brya' },
    ]),
    lab: new StringListFilterCategoryBuilder()
      .setLabel('Lab')
      .setOptions([{ label: 'MTV', value: 'MTV' }]),
    state: new StringListFilterCategoryBuilder()
      .setLabel('State')
      .setOptions([{ label: 'READY', value: 'READY' }]),
  }) as unknown as Record<ChromeOSFilterKey, StringListFilterCategoryBuilder>;

describe('<PriorityRulesPanel />', () => {
  const mockCreateRule = jest.fn();
  const mockUpdateRule = jest.fn();
  const mockDeleteRule = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    mockCreateRule.mockResolvedValue({});
    mockUpdateRule.mockResolvedValue({});
    mockDeleteRule.mockResolvedValue({});

    jest
      .spyOn(UseChromeOSFiltersModule, 'useChromeOSFilterBuilders')
      .mockReturnValue({
        filterBuilders: createMockFilterBuilders(),
        isLoading: false,
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const setupMockHook = (
    rules: readonly PriorityRule[] = MOCK_RULES,
    isLoading = false,
  ) => {
    jest.spyOn(UsePriorityRulesModule, 'usePriorityRules').mockReturnValue({
      rules,
      isLoading,
      isError: false,
      error: null,
      refetch: jest.fn(),
      createRule: mockCreateRule,
      isCreating: false,
      createError: null,
      updateRule: mockUpdateRule,
      isUpdating: false,
      updateError: null,
      deleteRule: mockDeleteRule,
      isDeleting: false,
      deleteError: null,
    });
  };

  const renderPanel = () =>
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <PriorityRulesPanel />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

  it('renders panel title, rules list, and initial collapsed state with 3 visible rules', () => {
    setupMockHook();
    renderPanel();

    expect(
      screen.getByRole('heading', {
        level: 6,
        name: /Priority Scoring Rules/i,
      }),
    ).toBeInTheDocument();

    // Default visible rules = 3
    const row1 = screen.getByTestId('priority-rule-row-1');
    expect(within(row1).getByText(/Board/i)).toBeInTheDocument();

    const row2 = screen.getByTestId('priority-rule-row-2');
    expect(within(row2).getByText(/Pool/i)).toBeInTheDocument();

    const row3 = screen.getByTestId('priority-rule-row-3');
    expect(within(row3).getByText(/Model/i)).toBeInTheDocument();

    expect(screen.queryByTestId('priority-rule-row-4')).not.toBeInTheDocument();

    // Show 1 more rule button
    expect(screen.getByTestId('show-more-rules-button')).toHaveTextContent(
      'Show 1 more rule',
    );
  });

  it('expands and collapses rules list when clicking expand toggle', async () => {
    setupMockHook();
    renderPanel();

    const expandButton = screen.getByTestId('show-more-rules-button');
    fireEvent.click(expandButton);

    // Now all 4 rules should be visible
    const row4 = screen.getByTestId('priority-rule-row-4');
    expect(within(row4).getByText(/Lab/i)).toBeInTheDocument();
    expect(screen.getByTestId('show-less-rules-button')).toHaveTextContent(
      'Show less rules',
    );

    // Click collapse
    fireEvent.click(screen.getByTestId('show-less-rules-button'));
    expect(screen.queryByTestId('priority-rule-row-4')).not.toBeInTheDocument();
  });

  it('shows Apply button when an existing rule weight is modified and updates on click', async () => {
    setupMockHook();
    renderPanel();

    // Initially Apply button is not visible for pristine row
    expect(screen.queryByTestId('rule-apply-button-1')).not.toBeInTheDocument();

    // Edit weight for rule 1
    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '300' } });

    // Apply button appears
    const applyButton = screen.getByTestId('rule-apply-button-1');
    expect(applyButton).toBeInTheDocument();

    // Click Apply
    fireEvent.click(applyButton);

    await waitFor(() => {
      expect(mockUpdateRule).toHaveBeenCalledWith({
        id: '1',
        expressionAip160: 'board = "bria"',
        weight: '300',
      });
    });

    // After successful update, apply button is hidden
    await waitFor(() => {
      expect(
        screen.queryByTestId('rule-apply-button-1'),
      ).not.toBeInTheDocument();
    });
  });

  it('shows Apply button when filter chip is removed in FilterBar', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const row1 = screen.getByTestId('priority-rule-row-1');
    const deleteChipIcon = within(row1).getByTestId('CancelIcon');

    fireEvent.click(deleteChipIcon);

    // Apply button appears since filter is now dirty (cleared)
    await waitFor(() => {
      expect(screen.getByTestId('rule-apply-button-1')).toBeInTheDocument();
    });
  });

  it('allows adding a new rule draft and creates it on Apply', async () => {
    setupMockHook(MOCK_RULES.slice(0, 2)); // 2 existing rules
    renderPanel();
    const user = userEvent.setup();

    const addRuleButton = screen.getByTestId('add-priority-rule-button');
    fireEvent.click(addRuleButton);

    // New draft row rendered with FilterBar search input
    const rowDraft = screen.getAllByTestId(/^priority-rule-row-draft-/)[0];
    expect(rowDraft).toBeInTheDocument();

    const searchInput = within(rowDraft).getByPlaceholderText(
      'Add rule filter (e.g. pool, board, model)...',
    );
    expect(searchInput).toBeInTheDocument();

    // Type exact filter match in search bar and press Enter
    await user.type(searchInput, 'model:corsola');
    await user.keyboard('{Enter}');

    // Filter chip for Model should be added
    expect(within(rowDraft).getByText(/Model/i)).toBeInTheDocument();

    const weightInputs = screen.getAllByLabelText(/points weight/i);
    const draftWeightInput = weightInputs[weightInputs.length - 1];
    fireEvent.change(draftWeightInput, {
      target: { value: '750' },
    });

    const applyButtons = screen.getAllByRole('button', { name: /Apply/i });
    expect(applyButtons.length).toBeGreaterThan(0);
    fireEvent.click(applyButtons[applyButtons.length - 1]);

    await waitFor(() => {
      expect(mockCreateRule).toHaveBeenCalledWith({
        priorityRule: {
          id: '0',
          expressionAip160: '(model = "corsola")',
          weight: '750',
        },
      });
    });
  });

  it('disables "Add rule" button when 5 rules exist', () => {
    const fiveRules: readonly PriorityRule[] = [
      ...MOCK_RULES,
      {
        id: '5',
        expressionAip160: 'state = "READY"',
        weight: '10',
      },
    ];
    setupMockHook(fiveRules);
    renderPanel();

    const addRuleButton = screen.getByTestId('add-priority-rule-button');
    expect(addRuleButton).toBeDisabled();
    expect(screen.getByText('Limit of 5 rules reached')).toBeInTheDocument();
  });

  it('deletes an existing rule when clicking delete icon button', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const deleteButton = screen.getByLabelText('delete rule 1');
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteRule).toHaveBeenCalledWith({ id: '1' });
    });
  });

  it('deletes a draft row without calling API', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    // Add draft
    fireEvent.click(screen.getByTestId('add-priority-rule-button'));
    expect(screen.getByTestId(/^priority-rule-row-draft-/)).toBeInTheDocument();

    // Delete draft
    const deleteButtons = screen.getAllByLabelText(/delete rule/i);
    const draftDeleteButton = deleteButtons[deleteButtons.length - 1];
    fireEvent.click(draftDeleteButton);

    expect(
      screen.queryByTestId(/^priority-rule-row-draft-/),
    ).not.toBeInTheDocument();
    expect(mockDeleteRule).not.toHaveBeenCalled();
  });

  it('displays client-side validation errors for invalid weight or empty filter', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    // Set invalid weight > 1,000,000
    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '2000000' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    expect(
      await screen.findByText(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      ),
    ).toBeInTheDocument();
    expect(mockUpdateRule).not.toHaveBeenCalled();

    // Clear filter chip
    const row1 = screen.getByTestId('priority-rule-row-1');
    const deleteChipIcon = within(row1).getByTestId('CancelIcon');
    fireEvent.click(deleteChipIcon);

    fireEvent.change(weightInput, { target: { value: '100' } });
    fireEvent.click(applyButton);

    expect(
      await screen.findByText('Filter expression cannot be empty'),
    ).toBeInTheDocument();
    expect(mockUpdateRule).not.toHaveBeenCalled();
  });

  it('rejects invalid non-integer weights such as alphanumeric values', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: 'abc' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    expect(
      await screen.findByText(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      ),
    ).toBeInTheDocument();
    expect(mockUpdateRule).not.toHaveBeenCalled();
  });

  it('preserves dirty row edits and additional drafts when one rule is applied', async () => {
    let currentRules: readonly PriorityRule[] = MOCK_RULES.slice(0, 2);
    const mockUpdate = jest.fn().mockImplementation(async () => {
      // Simulate remote rules query refetch after update
      currentRules = [
        {
          id: '1',
          expressionAip160: 'board = "bria"',
          weight: '999',
        },
        currentRules[1],
      ];
    });

    jest
      .spyOn(UsePriorityRulesModule, 'usePriorityRules')
      .mockImplementation(() => ({
        rules: currentRules,
        isLoading: false,
        isError: false,
        error: null,
        refetch: jest.fn(),
        createRule: mockCreateRule,
        isCreating: false,
        createError: null,
        updateRule: mockUpdate,
        isUpdating: false,
        updateError: null,
        deleteRule: mockDeleteRule,
        isDeleting: false,
        deleteError: null,
      }));

    renderPanel();
    const user = userEvent.setup();

    // Edit Rule 1
    const weightInput1 = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput1, { target: { value: '999' } });

    // Edit Rule 2
    const weightInput2 = screen.getByTestId('rule-weight-input-2');
    fireEvent.change(weightInput2, { target: { value: '777' } });

    // Add a draft
    const addRuleButton = screen.getByTestId('add-priority-rule-button');
    fireEvent.click(addRuleButton);

    const rowDraft = screen.getAllByTestId(/^priority-rule-row-draft-/)[0];
    const draftSearchInput = within(rowDraft).getByPlaceholderText(
      'Add rule filter (e.g. pool, board, model)...',
    );
    await user.type(draftSearchInput, 'model:brya');
    await user.keyboard('{Enter}');

    // Apply Rule 1
    const applyButton1 = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton1);

    await waitFor(() => {
      expect(mockUpdate).toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(
        screen.queryByTestId('rule-apply-button-1'),
      ).not.toBeInTheDocument();
    });

    // Rule 2's dirty weight (777) and the draft row with 'Model' chip must still exist!
    expect(screen.getByDisplayValue('777')).toBeInTheDocument();
    await waitFor(() => {
      const activeDraftRow = screen.getByTestId(/^priority-rule-row-draft-/);
      expect(within(activeDraftRow).getByText(/Model/i)).toBeInTheDocument();
    });
  });

  it('displays server validation error on mutation rejection', async () => {
    mockUpdateRule.mockRejectedValueOnce(
      new Error('invalid AIP-160 filter expression: syntax error'),
    );
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '300' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    expect(
      await screen.findByText(
        'invalid AIP-160 filter expression: syntax error',
      ),
    ).toBeInTheDocument();
  });

  it('renders empty state when there are no priority rules', () => {
    setupMockHook([]);
    renderPanel();

    expect(
      screen.getByText(
        /No priority scoring rules configured. Click “\+ Add rule” below to create your first rule./i,
      ),
    ).toBeInTheDocument();
  });

  it('renders loading spinner when loading priority rules', () => {
    setupMockHook([], true);
    renderPanel();

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('renders query error alert when fetching rules fails', () => {
    jest.spyOn(UsePriorityRulesModule, 'usePriorityRules').mockReturnValue({
      rules: [],
      isLoading: false,
      isError: true,
      error: new Error('Failed to load rules from backend'),
      refetch: jest.fn(),
      createRule: mockCreateRule,
      isCreating: false,
      createError: null,
      updateRule: mockUpdateRule,
      isUpdating: false,
      updateError: null,
      deleteRule: mockDeleteRule,
      isDeleting: false,
      deleteError: null,
    });
    renderPanel();

    expect(
      screen.getByText('Failed to load rules from backend'),
    ).toBeInTheDocument();
  });

  it('submits modified rule when pressing Enter in weight input', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '500' } });
    fireEvent.keyDown(weightInput, { key: 'Enter', code: 'Enter' });

    await waitFor(() => {
      expect(mockUpdateRule).toHaveBeenCalledWith({
        id: '1',
        expressionAip160: 'board = "bria"',
        weight: '500',
      });
    });
  });

  it('does not submit on Enter key when submission is already in progress', () => {
    jest.spyOn(UsePriorityRulesModule, 'usePriorityRules').mockReturnValue({
      rules: MOCK_RULES.slice(0, 1),
      isLoading: false,
      isError: false,
      error: null,
      refetch: jest.fn(),
      createRule: mockCreateRule,
      isCreating: false,
      createError: null,
      updateRule: mockUpdateRule,
      isUpdating: true,
      updateError: null,
      deleteRule: mockDeleteRule,
      isDeleting: false,
      deleteError: null,
    });
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.keyDown(weightInput, { key: 'Enter', code: 'Enter' });

    expect(mockUpdateRule).not.toHaveBeenCalled();
  });

  it('rejects decimal numbers as invalid non-integer weights', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '12.34' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    expect(
      await screen.findByText(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      ),
    ).toBeInTheDocument();
    expect(mockUpdateRule).not.toHaveBeenCalled();
  });

  it('displays error message when delete mutation fails', async () => {
    mockDeleteRule.mockRejectedValueOnce(
      new Error('Permission denied to delete rule'),
    );
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const deleteButton = screen.getByLabelText('delete rule 1');
    fireEvent.click(deleteButton);

    expect(
      await screen.findByText('Permission denied to delete rule'),
    ).toBeInTheDocument();
  });

  it('accepts valid boundary weights (-1000000 and 1000000)', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '-1000000' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    await waitFor(() => {
      expect(mockUpdateRule).toHaveBeenCalledWith({
        id: '1',
        expressionAip160: 'board = "bria"',
        weight: '-1000000',
      });
    });
  });

  it('rejects weights beyond boundaries (-1,000,001 and 1,000,001)', async () => {
    setupMockHook(MOCK_RULES.slice(0, 1));
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    fireEvent.change(weightInput, { target: { value: '-1000001' } });

    const applyButton = screen.getByTestId('rule-apply-button-1');
    fireEvent.click(applyButton);

    expect(
      await screen.findByText(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      ),
    ).toBeInTheDocument();
    expect(mockUpdateRule).not.toHaveBeenCalled();
  });
});
