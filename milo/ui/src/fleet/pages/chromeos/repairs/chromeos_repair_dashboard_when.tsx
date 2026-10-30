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

import { screen, waitFor, within } from '@testing-library/react';

import { RepairLab } from '@/fleet/pages/chromeos/repairs/use_repair_lab';
import {
  fleetFilterBar,
  fleetTable,
  genericWhen,
  getUser,
} from '@/fleet/testing_tools/integration/fleet_integration_test';

import { ChromeOSRepairDashboard } from './chromeos_repair_dashboard';

const RULE_FILTER_PLACEHOLDER =
  'Add rule filter (e.g. pool, model, pool health)...';

export const when = {
  ...genericWhen,

  dashboardIsRendered() {
    return genericWhen.renderPage(<ChromeOSRepairDashboard />);
  },

  async userGoesToNextPage() {
    await fleetTable.goToNextPage(getUser());
  },

  async userClaimsTask(dutId: string) {
    const row = await fleetTable.findRowByText(dutId);
    await getUser().click(
      fleetTable.withinRow(row).getByRole('button', { name: 'Claim' }),
    );
  },

  async userUnclaimsTask(dutId: string) {
    const row = await fleetTable.findRowByText(dutId);
    await getUser().click(
      fleetTable
        .withinRow(row)
        .getByLabelText('Assigned to you (click to unclaim)'),
    );
  },

  async userReassignsTaskFrom(dutId: string, previousAssignee: string) {
    const row = await fleetTable.findRowByText(dutId);
    await getUser().click(
      fleetTable
        .withinRow(row)
        .getByLabelText(
          `Assigned to ${previousAssignee} (click to assign to yourself)`,
        ),
    );
  },

  async userHoversScoreCell(dutId: string) {
    const row = await fleetTable.findRowByText(dutId);
    const scoreCell = fleetTable.getCell(row, 'Priority Score');
    const scoreTrigger = within(scoreCell).getByText(/pts$/i);
    await getUser().hover(scoreTrigger);
  },

  async userClicksAddRule() {
    await getUser().click(
      await screen.findByTestId('add-priority-rule-button'),
    );
  },

  async userTypesDraftRuleFilter(filterText: string) {
    const user = getUser();
    const draftRow = await screen.findByTestId(/^priority-rule-row-draft-/);
    await fleetFilterBar.applyFilter(user, filterText, {
      placeholder: RULE_FILTER_PLACEHOLDER,
      container: draftRow,
    });
  },

  async userSelectsDraftRangeFilter(
    filterLabel: string,
    range: { min?: string; max?: string },
  ) {
    const user = getUser();
    const draftRow = await screen.findByTestId(/^priority-rule-row-draft-/);
    const input = within(draftRow).getByPlaceholderText(
      RULE_FILTER_PLACEHOLDER,
    );
    await user.click(input);
    const menuItem = await screen.findByRole('menuitem', {
      name: new RegExp(`^${filterLabel}$`, 'i'),
    });
    await user.click(menuItem);
    const rangeGroup = await screen.findByRole('group', {
      name: 'Range filter',
    });
    if (range.min !== undefined) {
      const minInput = within(rangeGroup).getByLabelText('Min');
      await user.clear(minInput);
      await user.type(minInput, range.min);
    }
    if (range.max !== undefined) {
      const maxInput = within(rangeGroup).getByLabelText('Max');
      await user.clear(maxInput);
      await user.type(maxInput, range.max);
    }
    const popoverRoot = rangeGroup.parentElement;
    if (!popoverRoot) {
      throw new Error('Could not locate range filter dropdown container');
    }
    await user.click(
      within(popoverRoot).getByRole('button', { name: 'Apply' }),
    );
  },

  async userTypesDraftRuleWeight(weight: string) {
    const user = getUser();
    const draftRow = await screen.findByTestId(/^priority-rule-row-draft-/);
    const draftWeightInput = within(draftRow).getByLabelText(/points weight/i);
    await user.clear(draftWeightInput);
    await user.type(draftWeightInput, weight);
  },

  async userClicksDraftRuleApply() {
    const user = getUser();
    const draftRow = await screen.findByTestId(/^priority-rule-row-draft-/);
    const applyButton = within(draftRow).getByRole('button', {
      name: /Apply/i,
    });
    await waitFor(() => expect(applyButton).toBeEnabled());
    await user.click(applyButton);
  },

  async userClicksDraftRuleCancel() {
    const user = getUser();
    const draftRow = await screen.findByTestId(/^priority-rule-row-draft-/);
    const cancelButton = within(draftRow).getByRole('button', {
      name: /Cancel/i,
    });
    await user.click(cancelButton);
  },

  async userTypesRuleWeight(ruleId: string, weight: string) {
    const user = getUser();
    const weightInput = screen.getByTestId(`rule-weight-input-${ruleId}`);
    await user.clear(weightInput);
    await user.type(weightInput, weight);
  },

  async userSubmitsRuleWeightWithEnter(ruleId: string, weight: string) {
    const user = getUser();
    const weightInput = screen.getByTestId(`rule-weight-input-${ruleId}`);
    await user.clear(weightInput);
    await user.type(weightInput, `${weight}{Enter}`);
  },

  async userTypesRuleFilter(ruleId: string, filterText: string) {
    const user = getUser();
    const ruleRow = await screen.findByTestId(`priority-rule-row-${ruleId}`);
    await fleetFilterBar.applyFilter(user, filterText, {
      placeholder: RULE_FILTER_PLACEHOLDER,
      container: ruleRow,
    });
  },

  async userClearsRuleFilter(ruleId: string) {
    const ruleRow = await screen.findByTestId(`priority-rule-row-${ruleId}`);
    await getUser().click(within(ruleRow).getByTestId('CancelIcon'));
  },

  async userClicksRuleApply(ruleId: string) {
    const user = getUser();
    const applyButton = await screen.findByTestId(
      `rule-apply-button-${ruleId}`,
    );
    await waitFor(() => expect(applyButton).toBeEnabled());
    await user.click(applyButton);
  },

  async userClicksRuleCancel(ruleId: string) {
    const user = getUser();
    const cancelButton = await screen.findByTestId(
      `rule-cancel-button-${ruleId}`,
    );
    await user.click(cancelButton);
  },

  async userClicksDeleteRule(ruleId: string) {
    await getUser().click(screen.getByTestId(`rule-delete-button-${ruleId}`));
  },

  async userConfirmsDeleteRule(ruleId: string) {
    await getUser().click(
      screen.getByTestId(`rule-confirm-delete-button-${ruleId}`),
    );
  },

  async userCancelsDeleteRule(ruleId: string) {
    await getUser().click(screen.getByTestId(`rule-cancel-button-${ruleId}`));
  },

  async userPressesEscape() {
    await getUser().keyboard('{Escape}');
  },

  async userUpdatesRuleWeight(ruleId: string, weight: string) {
    await this.userTypesRuleWeight(ruleId, weight);
    await this.userClicksRuleApply(ruleId);
  },

  async userUpdatesRuleFilter(ruleId: string, filterText: string) {
    await this.userTypesRuleFilter(ruleId, filterText);
    await this.userClicksRuleApply(ruleId);
  },

  async userCreatesRule(options: { filterText?: string; weight: string }) {
    if (!screen.queryByTestId(/^priority-rule-row-draft-/)) {
      await this.userClicksAddRule();
    }
    if (options.filterText) {
      await this.userTypesDraftRuleFilter(options.filterText);
    }
    await this.userTypesDraftRuleWeight(options.weight);
    await this.userClicksDraftRuleApply();
  },

  async userDeletesRule(ruleId: string) {
    await this.userClicksDeleteRule(ruleId);
    await this.userConfirmsDeleteRule(ruleId);
  },

  async userExpandsAllRules() {
    await getUser().click(await screen.findByTestId('show-more-rules-button'));
  },

  async userCollapsesRules() {
    await getUser().click(await screen.findByTestId('show-less-rules-button'));
  },

  async userOpensWorkforceActivity() {
    await genericWhen.userClicksButton(/Workforce Activity/i);
  },

  async userSelectsLab(lab: RepairLab) {
    const user = getUser();
    await user.click(await screen.findByTestId('repair-lab-select'));
    await user.click(await screen.findByTestId(`repair-lab-option-${lab}`));
  },

  async userOpensIrmFilterDialog(bugId: string) {
    const user = getUser();
    const link = await screen.findByRole('link', { name: `b/${bugId}` });
    const row = link.closest('tr');
    if (!row) {
      throw new Error(`Could not locate table row for IRM bug b/${bugId}`);
    }
    await user.click(within(row).getByTestId('irm-filters-btn'));
    await screen.findByTestId('irm-filter-dialog');
  },

  async userHoversIrmFilterBadge(bugId: string) {
    const link = await screen.findByRole('link', { name: `b/${bugId}` });
    const row = link.closest('tr');
    if (!row) {
      throw new Error(`Could not locate table row for IRM bug b/${bugId}`);
    }
    await getUser().hover(within(row).getByTestId('irm-filters-btn'));
  },

  async userClicksIrmAddFilterRow() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    await getUser().click(within(dialog).getByTestId('irm-filter-add-row'));
  },

  async userTypesIrmFilterExpression(filterText: string) {
    const user = getUser();
    const dialog = await screen.findByTestId('irm-filter-dialog');
    const filterRow = await within(dialog).findByTestId('irm-filter-row');
    await fleetFilterBar.applyFilter(user, filterText, {
      placeholder: 'Filter affected devices by board, model, pool...',
      container: filterRow,
    });
  },

  async userClicksIrmMatchAllDevices() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    await getUser().click(within(dialog).getByTestId('irm-filter-all-btn'));
  },

  async userClicksIrmDeleteRow(index = 0) {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    const deleteButtons = within(dialog).getAllByTestId(
      'irm-filter-delete-btn',
    );
    await getUser().click(deleteButtons[index]);
  },

  async userClicksIrmDialogSave() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    await getUser().click(within(dialog).getByTestId('irm-filter-save'));
  },

  async userClicksIrmDialogCancel() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    await getUser().click(within(dialog).getByTestId('irm-filter-cancel'));
  },
};
