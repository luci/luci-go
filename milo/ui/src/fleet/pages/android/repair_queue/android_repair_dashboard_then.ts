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
  fleetFeedback,
  fleetTable,
  genericThen,
} from '@/fleet/testing_tools/integration/fleet_integration_test';
import { AndroidPageWorkspace } from '@/fleet/workspaces';

export interface ExpectedAndroidRepairTaskRow {
  rank?: string | number;
  machineType?: string;
  lab?: string;
  hostGroup?: string;
  runTarget?: string;
  health?: string | RegExp;
  score?: string;
  claimable?: boolean;
  claimed?: boolean;
}

export const then = {
  ...genericThen,

  dashboardHeadersAreVisible(workspace: AndroidPageWorkspace = 'Android') {
    genericThen.headingIsVisible(`${workspace} Manual Repair Dashboard`, 4);
    genericThen.headingIsVisible(/Priority Scoring Rules/i, 6);
    genericThen.headingIsVisible('Prioritized Manual Repair Queue', 6);
  },

  async repairTaskMatches(
    deviceId: string,
    expected: ExpectedAndroidRepairTaskRow,
  ) {
    const row = await fleetTable.findRowByText(deviceId);
    const rowScope = fleetTable.withinRow(row);
    if (expected.rank !== undefined) {
      fleetTable.expectCellText(row, 'Rank', String(expected.rank));
    }
    if (expected.machineType !== undefined) {
      fleetTable.expectCellText(row, 'Machine Type', expected.machineType);
    }
    if (expected.lab !== undefined) {
      fleetTable.expectCellText(row, 'Lab', expected.lab);
    }
    if (expected.hostGroup !== undefined) {
      fleetTable.expectCellText(row, 'Host Group', expected.hostGroup);
    }
    if (expected.runTarget !== undefined) {
      fleetTable.expectCellText(row, 'Run Target', expected.runTarget);
    }
    if (expected.health !== undefined) {
      fleetTable.expectCellText(
        row,
        'Host Group / Run Target Health',
        expected.health,
      );
    }
    if (expected.score !== undefined) {
      fleetTable.expectCellText(row, 'Priority Score', expected.score);
    }
    if (expected.claimable) {
      expect(rowScope.getByRole('button', { name: 'Claim' })).toBeEnabled();
    }
    if (expected.claimed === true) {
      expect(
        await rowScope.findByLabelText('Assigned to you (click to unclaim)'),
      ).toBeInTheDocument();
    }
    if (expected.claimed === false) {
      expect(
        await rowScope.findByRole('button', { name: 'Claim' }),
      ).toBeInTheDocument();
      expect(
        rowScope.queryByLabelText('Assigned to you (click to unclaim)'),
      ).not.toBeInTheDocument();
    }
  },

  async repairTaskIsClaimed(deviceId: string) {
    const row = await fleetTable.findRowByText(deviceId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByLabelText('Assigned to you (click to unclaim)'),
    ).toBeInTheDocument();
    expect(
      rowScope.queryByRole('button', { name: 'Claim' }),
    ).not.toBeInTheDocument();
  },

  async repairTaskIsUnclaimed(deviceId: string) {
    const row = await fleetTable.findRowByText(deviceId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByRole('button', { name: 'Claim' }),
    ).toBeInTheDocument();
    expect(
      rowScope.queryByLabelText('Assigned to you (click to unclaim)'),
    ).not.toBeInTheDocument();
  },

  async repairTaskIsClaimedByOther(deviceId: string, email: string) {
    const row = await fleetTable.findRowByText(deviceId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByLabelText(
        `Assigned to ${email} (click to assign to yourself)`,
      ),
    ).toBeInTheDocument();
  },

  async repairTaskIsPinnedWithRank(deviceId: string, rank: number) {
    const row = await fleetTable.findRowByText(deviceId);
    expect(
      fleetTable.withinRow(row).getByLabelText(`Rank ${rank}, pinned to top`),
    ).toBeInTheDocument();
  },

  async scoreBreakdownShowsRule(
    ruleId: string,
    expectedSummaryText: string | RegExp,
  ) {
    expect(
      await screen.findByTestId(`matched-rule-${ruleId}`),
    ).toBeInTheDocument();
    expect(await screen.findByText(expectedSummaryText)).toBeInTheDocument();
  },

  async scoreBreakdownShowsTotal(expectedTotalText: string) {
    expect(await screen.findByText('Total score')).toBeInTheDocument();
    expect(await screen.findByText(expectedTotalText)).toBeInTheDocument();
  },

  async scoreBreakdownShowsEmpty() {
    expect(
      await screen.findByText('No matched priority rules (0 pts)'),
    ).toBeInTheDocument();
  },

  async tableIsEmpty() {
    await fleetTable.expectEmpty();
  },

  async errorAlertIsVisible(message: string | RegExp) {
    await fleetFeedback.expectErrorAlert(message);
  },

  async ruleExists(ruleId: string) {
    expect(
      await screen.findByTestId(`priority-rule-row-${ruleId}`),
    ).toBeInTheDocument();
  },

  async ruleDoesNotExist(ruleId: string) {
    await waitFor(() => {
      expect(
        screen.queryByTestId(`priority-rule-row-${ruleId}`),
      ).not.toBeInTheDocument();
    });
  },

  async ruleHasWeight(ruleId: string, expectedWeight: number) {
    await waitFor(() => {
      expect(screen.getByTestId(`rule-weight-input-${ruleId}`)).toHaveValue(
        expectedWeight,
      );
    });
  },

  async ruleHasFilterText(ruleId: string, expectedText: string | RegExp) {
    const row = await screen.findByTestId(`priority-rule-row-${ruleId}`);
    expect(await within(row).findByText(expectedText)).toBeInTheDocument();
  },

  async ruleRowShowsApplyCancel(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-apply-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    ).toBeInTheDocument();
  },

  async ruleRowShowsConfirmDelete(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-confirm-delete-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    ).toBeInTheDocument();
  },

  async ruleRowShowsDeleteIcon(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-delete-button-${ruleId}`),
    ).toBeInTheDocument();
  },

  async draftRuleRowIsVisible() {
    expect(
      await screen.findByTestId(/^priority-rule-row-draft-/),
    ).toBeInTheDocument();
  },

  draftRuleRowIsNotVisible() {
    expect(
      screen.queryByTestId(/^priority-rule-row-draft-/),
    ).not.toBeInTheDocument();
  },

  async validationErrorIsVisible(message: string | RegExp) {
    expect(await screen.findByText(message)).toBeInTheDocument();
  },

  async addRuleButtonIsDisabledWithLimit(maxRules = 15) {
    const addButton = await screen.findByTestId('add-priority-rule-button');
    expect(addButton).toBeDisabled();
    expect(addButton).toHaveTextContent(`Limit of ${maxRules} rules reached`);
  },

  async ruleEditingIsDisabled(ruleId: string) {
    expect(
      await screen.findByTestId(`priority-rule-row-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('add-priority-rule-button'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`rule-delete-button-${ruleId}`),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId(`rule-weight-input-${ruleId}`)).toBeDisabled();
  },
};
