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
  REPAIR_LAB_LOCAL_STORAGE_KEY,
  RepairLab,
} from '@/fleet/pages/chromeos/repairs/use_repair_lab';
import {
  fleetFeedback,
  fleetTable,
  genericThen,
} from '@/fleet/testing_tools/integration/fleet_integration_test';

export interface ExpectedRepairTaskRow {
  rank?: string | number;
  taskLink?: string;
  pool?: string;
  model?: string;
  health?: string | RegExp;
  score?: string;
  activeIrmBugId?: string;
  claimable?: boolean;
  claimed?: boolean;
}

export const then = {
  ...genericThen,

  dashboardHeadersAreVisible() {
    genericThen.headingIsVisible('ChromeOS Manual Repair Dashboard', 4);
    genericThen.headingIsVisible(/Priority Scoring Rules/i, 6);
    genericThen.headingIsVisible('Prioritized Manual Repair Queue', 6);
  },

  async repairTaskMatches(dutId: string, expected: ExpectedRepairTaskRow) {
    const row = await fleetTable.findRowByText(dutId);
    const rowScope = fleetTable.withinRow(row);
    if (expected.rank !== undefined) {
      fleetTable.expectCellText(row, 'Rank', String(expected.rank));
    }
    if (expected.taskLink !== undefined) {
      expect(rowScope.getByRole('link', { name: dutId })).toHaveAttribute(
        'href',
        expected.taskLink,
      );
    }
    if (expected.pool !== undefined) {
      fleetTable.expectCellText(row, 'Pool', expected.pool);
    }
    if (expected.model !== undefined) {
      fleetTable.expectCellText(row, 'Model', expected.model);
    }
    if (expected.health !== undefined) {
      fleetTable.expectCellText(row, 'Pool / Model Health', expected.health);
    }
    if (expected.activeIrmBugId !== undefined) {
      fleetTable.expectCellText(
        row,
        'Active IRM',
        `b/${expected.activeIrmBugId}`,
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

  async repairTaskIsClaimed(dutId: string) {
    const row = await fleetTable.findRowByText(dutId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByLabelText('Assigned to you (click to unclaim)'),
    ).toBeInTheDocument();
    expect(
      rowScope.queryByRole('button', { name: 'Claim' }),
    ).not.toBeInTheDocument();
  },

  async repairTaskIsUnclaimed(dutId: string) {
    const row = await fleetTable.findRowByText(dutId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByRole('button', { name: 'Claim' }),
    ).toBeInTheDocument();
    expect(
      rowScope.queryByLabelText('Assigned to you (click to unclaim)'),
    ).not.toBeInTheDocument();
  },

  async repairTaskIsClaimedByOther(dutId: string, email: string) {
    const row = await fleetTable.findRowByText(dutId);
    const rowScope = fleetTable.withinRow(row);
    expect(
      await rowScope.findByLabelText(
        `Assigned to ${email} (click to assign to yourself)`,
      ),
    ).toBeInTheDocument();
  },

  async scoreBreakdownShowsRule(
    ruleId: string,
    expectedSummaryText: string | RegExp,
  ) {
    const matchedRule = await screen.findByTestId(`matched-rule-${ruleId}`);
    expect(matchedRule).toBeInTheDocument();
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

  async scoreBreakdownShowsUnknownRule(ruleId: string) {
    expect(await screen.findByText(`Rule #${ruleId}`)).toBeInTheDocument();
  },

  async repairTaskHasRank(dutId: string, rank: string | number) {
    const row = await fleetTable.findRowByText(dutId);
    fleetTable.expectCellText(row, 'Rank', String(rank));
  },

  async repairTaskHasPeripherals(
    dutId: string,
    expectedLabels: readonly string[],
  ) {
    const row = await fleetTable.findRowByText(dutId);
    const cell = fleetTable.getCell(row, 'Peripherals');
    for (const label of expectedLabels) {
      expect(within(cell).getByLabelText(label)).toBeInTheDocument();
    }
  },

  async repairTaskHasMissingIdFallback(rowText: string) {
    const row = await fleetTable.findRowByText(rowText);
    fleetTable.expectCellText(row, 'ID', '-');
    const idCell = fleetTable.getCell(row, 'ID');
    expect(within(idCell).queryByRole('link')).not.toBeInTheDocument();
  },

  async repairTaskHasSingleIrmBadge(dutId: string, bugId: string) {
    const row = await fleetTable.findRowByText(dutId);
    const cell = fleetTable.getCell(row, 'Active IRM');
    expect(within(cell).getAllByText(`b/${bugId}`)).toHaveLength(1);
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

  async ruleHasWeight(ruleId: string, weight: number) {
    await waitFor(() => {
      expect(
        screen.queryByTestId(`rule-apply-button-${ruleId}`),
      ).not.toBeInTheDocument();
    });
    expect(screen.getByTestId(`rule-weight-input-${ruleId}`)).toHaveValue(
      weight,
    );
  },

  async ruleHasWeightInputValue(ruleId: string, weight: number) {
    await waitFor(() => {
      expect(screen.getByTestId(`rule-weight-input-${ruleId}`)).toHaveValue(
        weight,
      );
    });
  },

  async ruleHasFilterText(ruleId: string, expectedText: string | RegExp) {
    await waitFor(() => {
      expect(
        screen.queryByTestId(`rule-apply-button-${ruleId}`),
      ).not.toBeInTheDocument();
    });
    const ruleRow = await screen.findByTestId(`priority-rule-row-${ruleId}`);
    expect(within(ruleRow).getByText(expectedText)).toBeInTheDocument();
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

  async irmIncidentIsVisible(
    bugId: string,
    title: string,
    options?: { filterLabel?: string | RegExp; affectedCount?: number },
  ) {
    const chip = await screen.findByRole('link', { name: `b/${bugId}` });
    expect(chip).toHaveAttribute(
      'href',
      `https://b.corp.google.com/issues/${bugId}`,
    );
    expect(await screen.findByText(title)).toBeInTheDocument();
    const row = chip.closest('tr');
    if (row && options?.filterLabel !== undefined) {
      expect(within(row).getByTestId('irm-filters-btn')).toHaveTextContent(
        options.filterLabel,
      );
    }
    if (row && options?.affectedCount !== undefined) {
      expect(within(row).getByTestId('irm-affected-count')).toHaveTextContent(
        String(options.affectedCount),
      );
    }
  },

  async selectedLabIs(lab: RepairLab) {
    expect(await screen.findByTestId('repair-lab-select')).toHaveTextContent(
      `Lab: ${lab}`,
    );
  },

  storedRepairLabIs(lab: RepairLab) {
    expect(window.localStorage.getItem(REPAIR_LAB_LOCAL_STORAGE_KEY)).toBe(lab);
  },

  async irmIncidentWithoutBugFallbackIsVisible() {
    expect(await screen.findByText('N/A')).toBeInTheDocument();
    expect(await screen.findByText('—')).toBeInTheDocument();
  },

  async priorityRulesShowsError(
    expectedText: string | RegExp = /Priority rules service unavailable/i,
  ) {
    const alert = await screen.findByText(expectedText);
    expect(alert.closest('[role="alert"]')).toBeInTheDocument();
  },

  async irmTableIsEmpty() {
    expect(await screen.findByTestId('active-irm-empty')).toHaveTextContent(
      'No active IRM incidents',
    );
  },

  async irmTableShowsError() {
    const alert = await screen.findByTestId('active-irm-error');
    expect(
      within(alert).getByText(
        /The service for loading active IRM incidents is currently unavailable/i,
      ),
    ).toBeInTheDocument();
  },

  async repairQueueShowsError() {
    await fleetFeedback.expectErrorAlert(
      'Error Loading Repair Queue',
      /The service for fetch repair queue is currently unavailable/i,
    );
  },

  async workforceActivityShowsInProgress(count: number) {
    const activityBtn = await screen.findByRole('button', {
      name: /Workforce Activity/i,
    });
    expect(
      await within(activityBtn).findByText(`${count} In-Progress`),
    ).toBeInTheDocument();
  },

  workforceActivityIsNotVisible() {
    expect(
      screen.queryByRole('button', { name: /Workforce Activity/i }),
    ).not.toBeInTheDocument();
  },

  async irmIncidentIsSyncing(bugId: string) {
    const link = await screen.findByRole('link', { name: `b/${bugId}` });
    const row = link.closest('tr');
    if (!row) {
      throw new Error(`Could not locate table row for IRM bug b/${bugId}`);
    }
    expect(
      await within(row).findByTestId('irm-sync-spinner'),
    ).toBeInTheDocument();
    expect(within(row).getByTestId('irm-filters-btn')).toBeDisabled();
  },

  async irmMutationErrorIsVisible(expectedText?: string | RegExp) {
    const alert = await screen.findByTestId('active-irm-mutation-error');
    expect(alert).toBeInTheDocument();
    if (expectedText !== undefined) {
      expect(alert).toHaveTextContent(expectedText);
    }
  },

  async irmFilterTooltipContains(expectedExpressions: readonly string[]) {
    for (const expr of expectedExpressions) {
      expect(await screen.findByText(expr)).toBeInTheDocument();
    }
  },

  async validationErrorIsVisible(expectedMessage: string | RegExp) {
    const errorEl = await screen.findByText(expectedMessage);
    expect(errorEl.closest('[role="alert"]')).toBeInTheDocument();
  },

  async irmFilterValidationErrorIsVisible(expectedMessage: string | RegExp) {
    const alert = await screen.findByTestId('irm-filter-error');
    expect(alert).toHaveTextContent(expectedMessage);
  },

  async irmIncidentHasNoFiltersReadOnly(bugId: string) {
    const link = await screen.findByRole('link', { name: `b/${bugId}` });
    const row = link.closest('tr');
    if (!row) {
      throw new Error(`Could not locate table row for IRM bug b/${bugId}`);
    }
    expect(await within(row).findByTestId('irm-no-filters')).toHaveTextContent(
      'None',
    );
    expect(
      within(row).queryByTestId('irm-filters-btn'),
    ).not.toBeInTheDocument();
  },

  async irmFilterDialogIsReadOnly() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    expect(
      within(dialog).getByText('View Incident Filters'),
    ).toBeInTheDocument();
    expect(within(dialog).getByTestId('irm-filter-close')).toBeInTheDocument();
    expect(
      within(dialog).queryByTestId('irm-filter-save'),
    ).not.toBeInTheDocument();
    expect(
      within(dialog).queryByTestId('irm-filter-add-row'),
    ).not.toBeInTheDocument();
    expect(
      within(dialog).queryByTestId('irm-filter-all-btn'),
    ).not.toBeInTheDocument();
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

  async ruleRowShowsApplyCancel(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-apply-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId(`rule-delete-button-${ruleId}`),
    ).not.toBeInTheDocument();
  },

  async ruleRowShowsDeleteIcon(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-delete-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId(`rule-apply-button-${ruleId}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`rule-cancel-button-${ruleId}`),
    ).not.toBeInTheDocument();
  },

  async ruleRowShowsConfirmDelete(ruleId: string) {
    expect(
      await screen.findByTestId(`rule-confirm-delete-button-${ruleId}`),
    ).toBeInTheDocument();
    expect(
      await screen.findByTestId(`rule-cancel-button-${ruleId}`),
    ).toBeInTheDocument();
  },

  async irmDialogShowsMatchAllRow() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    expect(
      within(dialog).getByText('Matches all devices across the fleet'),
    ).toBeInTheDocument();
    expect(within(dialog).getByTestId('irm-filter-add-row')).toBeDisabled();
    expect(within(dialog).getByTestId('irm-filter-all-btn')).toBeDisabled();
  },

  async irmDialogAddButtonsAreEnabled() {
    const dialog = await screen.findByTestId('irm-filter-dialog');
    expect(within(dialog).getByTestId('irm-filter-add-row')).toBeEnabled();
    expect(within(dialog).getByTestId('irm-filter-all-btn')).toBeEnabled();
  },

  irmDialogIsClosed() {
    expect(screen.queryByTestId('irm-filter-dialog')).not.toBeInTheDocument();
  },
};
