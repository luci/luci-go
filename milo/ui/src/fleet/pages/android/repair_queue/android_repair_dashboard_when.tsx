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

import {
  fleetFilterBar,
  fleetTable,
  genericWhen,
  getUser,
} from '@/fleet/testing_tools/integration/fleet_integration_test';
import { AndroidPageWorkspace } from '@/fleet/workspaces';

import { AndroidRepairDashboard } from './android_repair_dashboard';

const RULE_FILTER_PLACEHOLDER =
  'Add rule filter (e.g. state = "DYING" AND run_target = "seahawk")...';

export const when = {
  ...genericWhen,

  dashboardIsRendered(workspace: AndroidPageWorkspace = 'Android') {
    return genericWhen.renderPage(
      <AndroidRepairDashboard workspace={workspace} />,
    );
  },

  async userGoesToNextPage() {
    await fleetTable.goToNextPage(getUser());
  },

  async userClaimsTask(deviceId: string) {
    const row = await fleetTable.findRowByText(deviceId);
    await getUser().click(
      fleetTable.withinRow(row).getByRole('button', { name: 'Claim' }),
    );
  },

  async userUnclaimsTask(deviceId: string) {
    const row = await fleetTable.findRowByText(deviceId);
    await getUser().click(
      fleetTable
        .withinRow(row)
        .getByLabelText('Assigned to you (click to unclaim)'),
    );
  },

  async userReassignsTaskFrom(deviceId: string, previousAssignee: string) {
    const row = await fleetTable.findRowByText(deviceId);
    await getUser().click(
      fleetTable
        .withinRow(row)
        .getByLabelText(
          `Assigned to ${previousAssignee} (click to assign to yourself)`,
        ),
    );
  },

  async userHoversScoreCell(deviceId: string) {
    const row = await fleetTable.findRowByText(deviceId);
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
    await user.click(within(draftRow).getByRole('button', { name: /Cancel/i }));
  },

  async userTypesRuleWeight(ruleId: string, weight: string) {
    const user = getUser();
    const weightInput = screen.getByTestId(`rule-weight-input-${ruleId}`);
    await user.clear(weightInput);
    await user.type(weightInput, weight);
  },

  async userClicksRuleApply(ruleId: string) {
    const applyButton = await screen.findByTestId(
      `rule-apply-button-${ruleId}`,
    );
    await waitFor(() => expect(applyButton).toBeEnabled());
    await getUser().click(applyButton);
  },

  async userClicksRuleCancel(ruleId: string) {
    await getUser().click(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    );
  },

  async userClicksDeleteRule(ruleId: string) {
    await getUser().click(
      await screen.findByTestId(`rule-delete-button-${ruleId}`),
    );
  },

  async userConfirmsDeleteRule(ruleId: string) {
    await getUser().click(
      await screen.findByTestId(`rule-confirm-delete-button-${ruleId}`),
    );
  },

  async userCancelsDeleteRule(ruleId: string) {
    await getUser().click(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    );
  },

  async userTypesRuleFilter(ruleId: string, filterText: string) {
    const row = screen.getByTestId(`priority-rule-row-${ruleId}`);
    await fleetFilterBar.applyFilter(getUser(), filterText, {
      placeholder: RULE_FILTER_PLACEHOLDER,
      container: row,
    });
  },

  async userExpandsAllRules() {
    await getUser().click(await screen.findByTestId('show-more-rules-button'));
  },

  async userCollapsesRules() {
    await getUser().click(await screen.findByTestId('show-less-rules-button'));
  },
};
