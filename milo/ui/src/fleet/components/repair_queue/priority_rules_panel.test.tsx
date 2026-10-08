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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

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

  it('calls the update and delete callbacks', async () => {
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
    await waitFor(() => expect(deleteRule).toHaveBeenCalledWith({ id: '2' }));
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
});
