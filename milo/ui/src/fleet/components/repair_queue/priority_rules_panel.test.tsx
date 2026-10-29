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
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RepairPriorityRule } from './editable_rule_row';
import {
  PriorityRulesPanel,
  PriorityRulesPanelProps,
} from './priority_rules_panel';

const MOCK_RULES: readonly RepairPriorityRule[] = [
  { id: '1', expressionAip160: 'type = "PHYSICAL"', weight: '100' },
  { id: '2', expressionAip160: 'lab_name = "MTV"', weight: '-50' },
];

const FILTER_BUILDERS = {
  type: new StringListFilterCategoryBuilder()
    .setLabel('Type')
    .setOptions([{ label: 'PHYSICAL', value: 'PHYSICAL' }]),
  lab_name: new StringListFilterCategoryBuilder()
    .setLabel('Lab')
    .setOptions([{ label: 'MTV', value: 'MTV' }]),
};

describe('<PriorityRulesPanel />', () => {
  jest.setTimeout(25000);
  const createRule = jest.fn();
  const updateRule = jest.fn();
  const deleteRule = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    createRule.mockResolvedValue({});
    updateRule.mockResolvedValue({});
    deleteRule.mockResolvedValue({});
  });

  const renderPanel = (overrides: Partial<PriorityRulesPanelProps> = {}) =>
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <PriorityRulesPanel
            rules={MOCK_RULES}
            isLoading={false}
            isError={false}
            error={null}
            createRule={createRule}
            isCreating={false}
            updateRule={updateRule}
            isUpdating={false}
            deleteRule={deleteRule}
            isDeleting={false}
            filterBuilders={FILTER_BUILDERS}
            isBuildersLoading={false}
            canEdit
            {...overrides}
          />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

  it('renders the rules passed via props', () => {
    renderPanel();

    expect(screen.getByTestId('priority-rule-row-1')).toBeInTheDocument();
    expect(screen.getByTestId('priority-rule-row-2')).toBeInTheDocument();
    expect(screen.getByTestId('rule-weight-input-1')).toHaveValue(100);
    expect(screen.getByTestId('rule-weight-input-2')).toHaveValue(-50);
  });

  it('hides edit controls when canEdit is false', () => {
    renderPanel({ canEdit: false });

    expect(
      screen.queryByTestId('add-priority-rule-button'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('rule-delete-button-1'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('rule-weight-input-1')).toBeDisabled();
  });

  it('morphs trash icon into a single inline Delete button and removes the rule on confirm', async () => {
    renderPanel();

    fireEvent.change(screen.getByTestId('rule-weight-input-1'), {
      target: { value: '300' },
    });
    fireEvent.click(screen.getByTestId('rule-apply-button-1'));
    await waitFor(() =>
      expect(updateRule).toHaveBeenCalledWith({
        id: '1',
        expressionAip160: 'type = "PHYSICAL"',
        weight: '300',
      }),
    );

    fireEvent.click(screen.getByTestId('rule-delete-button-2'));
    expect(
      screen.queryByTestId('rule-delete-button-2'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('rule-apply-button-2')).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('rule-confirm-delete-button-2'));
    await waitFor(() => expect(deleteRule).toHaveBeenCalledWith({ id: '2' }));
    await waitFor(() =>
      expect(
        screen.queryByTestId('priority-rule-row-2'),
      ).not.toBeInTheDocument(),
    );
  });

  it('replaces Apply and trash icon with a single Delete button when a saved rule filter is emptied', async () => {
    renderPanel();
    const user = userEvent.setup({ delay: null });

    const row1 = screen.getByTestId('priority-rule-row-1');
    fireEvent.click(within(row1).getByTestId('CancelIcon'));

    await waitFor(() => {
      expect(
        screen.getByTestId('rule-confirm-delete-button-1'),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByTestId('rule-delete-button-1'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('rule-apply-button-1')).not.toBeInTheDocument();

    // Pressing Enter in weight input while filter is empty must not submit or show empty-filter error
    const weightInput1 = screen.getByTestId('rule-weight-input-1');
    fireEvent.keyDown(weightInput1, { key: 'Enter', code: 'Enter' });
    expect(updateRule).not.toHaveBeenCalled();
    expect(
      screen.queryByText('Filter expression cannot be empty'),
    ).not.toBeInTheDocument();

    // Adding a filter chip back restores Apply/Cancel (no trash icon while dirty), and clearing it again returns to Delete/Cancel
    const searchInput = within(row1).getByPlaceholderText('Add rule filter...');
    await user.type(searchInput, 'Lab:MTV{Enter}');
    await waitFor(() => {
      expect(screen.getByTestId('rule-apply-button-1')).toBeInTheDocument();
    });
    expect(screen.getByTestId('rule-cancel-button-1')).toBeInTheDocument();
    expect(
      screen.queryByTestId('rule-delete-button-1'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('rule-confirm-delete-button-1'),
    ).not.toBeInTheDocument();

    fireEvent.click(within(row1).getByTestId('CancelIcon'));
    await waitFor(() => {
      expect(
        screen.getByTestId('rule-confirm-delete-button-1'),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByTestId('rule-delete-button-1'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('rule-apply-button-1')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('rule-confirm-delete-button-1'));
    await waitFor(() => expect(deleteRule).toHaveBeenCalledWith({ id: '1' }));
  });

  it('shows the load error passed via props', () => {
    renderPanel({ isError: true, error: new Error('boom') });

    expect(screen.getByText('boom')).toBeInTheDocument();
  });

  it('uses a neutral filter placeholder unless one is provided', () => {
    const { unmount } = renderPanel();
    expect(
      screen.getAllByPlaceholderText('Add rule filter...').length,
    ).toBeGreaterThan(0);
    unmount();

    renderPanel({ searchPlaceholder: 'Custom placeholder' });
    expect(
      screen.getAllByPlaceholderText('Custom placeholder').length,
    ).toBeGreaterThan(0);
    expect(
      screen.queryByPlaceholderText('Add rule filter...'),
    ).not.toBeInTheDocument();
  });

  it('cancels edits on an existing rule and restores original values', () => {
    renderPanel();

    const weightInput = screen.getByTestId('rule-weight-input-1');
    expect(weightInput).toHaveValue(100);

    fireEvent.change(weightInput, { target: { value: '450' } });
    expect(weightInput).toHaveValue(450);
    expect(screen.getByTestId('rule-apply-button-1')).toBeInTheDocument();
    expect(screen.getByTestId('rule-cancel-button-1')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('rule-cancel-button-1'));

    expect(screen.getByTestId('rule-weight-input-1')).toHaveValue(100);
    expect(screen.queryByTestId('rule-apply-button-1')).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('rule-cancel-button-1'),
    ).not.toBeInTheDocument();
    expect(updateRule).not.toHaveBeenCalled();
  });

  it('cancels adding a draft rule and removes the draft row', () => {
    renderPanel();

    fireEvent.click(screen.getByTestId('add-priority-rule-button'));

    const draftWeightInput = screen.getByDisplayValue('0');
    expect(draftWeightInput).toBeInTheDocument();
    const cancelButton = screen.getAllByRole('button', { name: /cancel/i })[0];
    expect(cancelButton).toBeInTheDocument();

    fireEvent.click(cancelButton);

    expect(screen.queryByDisplayValue('0')).not.toBeInTheDocument();
    expect(createRule).not.toHaveBeenCalled();
  });

  it('cancels delete confirmation when clicking the Cancel button', () => {
    renderPanel();

    fireEvent.click(screen.getByTestId('rule-delete-button-2'));
    expect(
      screen.getByTestId('rule-confirm-delete-button-2'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('rule-cancel-button-2')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('rule-cancel-button-2'));

    expect(
      screen.queryByTestId('rule-confirm-delete-button-2'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('rule-delete-button-2')).toBeInTheDocument();
    expect(deleteRule).not.toHaveBeenCalled();
  });

  it('restores original rule when clicking Cancel after emptying all filter chips', async () => {
    renderPanel();

    const row1 = screen.getByTestId('priority-rule-row-1');
    fireEvent.click(within(row1).getByTestId('CancelIcon'));

    await waitFor(() => {
      expect(
        screen.getByTestId('rule-confirm-delete-button-1'),
      ).toBeInTheDocument();
    });
    expect(screen.getByTestId('rule-cancel-button-1')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('rule-cancel-button-1'));

    await waitFor(() => {
      expect(
        screen.queryByTestId('rule-confirm-delete-button-1'),
      ).not.toBeInTheDocument();
    });
    expect(screen.getByTestId('rule-delete-button-1')).toBeInTheDocument();
    expect(within(row1).getByText(/Type/i)).toBeInTheDocument();
    expect(deleteRule).not.toHaveBeenCalled();
  });

  it('expands and collapses rules list when more than 3 rules exist', () => {
    const fourRules: readonly RepairPriorityRule[] = [
      ...MOCK_RULES,
      { id: '3', expressionAip160: 'type = "PHYSICAL"', weight: '50' },
      { id: '4', expressionAip160: 'lab_name = "MTV"', weight: '25' },
    ];
    renderPanel({ rules: fourRules });

    expect(screen.queryByTestId('priority-rule-row-4')).not.toBeInTheDocument();
    expect(screen.getByTestId('show-more-rules-button')).toHaveTextContent(
      'Show 1 more rule',
    );

    fireEvent.click(screen.getByTestId('show-more-rules-button'));
    expect(screen.getByTestId('priority-rule-row-4')).toBeInTheDocument();
    expect(screen.getByTestId('show-less-rules-button')).toHaveTextContent(
      'Show less rules',
    );

    fireEvent.click(screen.getByTestId('show-less-rules-button'));
    expect(screen.queryByTestId('priority-rule-row-4')).not.toBeInTheDocument();
  });

  it('disables Add rule button when 15 rules exist', () => {
    const maxRules: readonly RepairPriorityRule[] = Array.from(
      { length: 15 },
      (_, i) => ({
        id: String(i + 1),
        expressionAip160: 'type = "PHYSICAL"',
        weight: '10',
      }),
    );
    renderPanel({ rules: maxRules });

    fireEvent.click(screen.getByTestId('show-more-rules-button'));
    expect(screen.getByTestId('add-priority-rule-button')).toBeDisabled();
    expect(screen.getByText('Limit of 15 rules reached')).toBeInTheDocument();
  });
});
