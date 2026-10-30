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
import { genericThen } from '@/fleet/testing_tools/integration/fleet_integration_test';

import { TimeframeOption } from './workforce_activity_view';

export interface ExpectedTechnicianRow {
  email?: string;
  scoreCleared?: string;
  avgPtsPerDut?: string;
  pickupRank?: string;
  completedRepairs?: string;
  mttr?: string;
  noClaimedDuts?: boolean;
}

const findChipForDut = async (dutId: string): Promise<HTMLElement> => {
  const chipLabel = await screen.findByText(new RegExp(dutId, 'i'));
  const chip = chipLabel.closest<HTMLElement>('.MuiChip-root');
  if (!chip) {
    throw new Error(`Could not locate MUI Chip for DUT ${dutId}`);
  }
  return chip;
};

export const then = {
  ...genericThen,

  headerIsVisible() {
    genericThen.headingIsVisible('Workforce & Technician Monitoring', 4);
  },

  async kpiCardIsVisible(title: string, value?: string) {
    const titleEl = await screen.findByText(title);
    const card = titleEl.closest<HTMLElement>('.MuiPaper-root');
    if (!card) {
      throw new Error(`Could not locate KPI card container for "${title}"`);
    }
    if (value !== undefined) {
      expect(await within(card).findByText(value)).toBeInTheDocument();
    }
  },

  kpiCardIsNotVisible(title: string) {
    expect(screen.queryByText(title)).not.toBeInTheDocument();
  },

  async technicianRowMatches(
    nameOrEmail: string,
    expected: ExpectedTechnicianRow,
  ) {
    const anchor = await screen.findByText(nameOrEmail);
    const row = anchor.closest<HTMLTableRowElement>('tr');
    if (!row) {
      throw new Error(`Could not locate table row for "${nameOrEmail}"`);
    }
    const rowScope = within(row);
    if (expected.email !== undefined) {
      expect(rowScope.getByText(expected.email)).toBeInTheDocument();
    }
    if (expected.scoreCleared !== undefined) {
      expect(rowScope.getByText(expected.scoreCleared)).toBeInTheDocument();
    }
    if (expected.avgPtsPerDut !== undefined) {
      expect(rowScope.getByText(expected.avgPtsPerDut)).toBeInTheDocument();
    }
    if (expected.pickupRank !== undefined) {
      expect(rowScope.getByText(expected.pickupRank)).toBeInTheDocument();
    }
    if (expected.completedRepairs !== undefined) {
      expect(rowScope.getByText(expected.completedRepairs)).toBeInTheDocument();
    }
    if (expected.mttr !== undefined) {
      expect(rowScope.getByText(expected.mttr)).toBeInTheDocument();
    }
    if (expected.noClaimedDuts) {
      expect(
        rowScope.getByText('No active devices claimed'),
      ).toBeInTheDocument();
    }
  },

  async technicianIsVisible(name: string) {
    expect(await screen.findByText(name)).toBeInTheDocument();
  },

  technicianIsNotVisible(name: string) {
    expect(screen.queryByText(name)).not.toBeInTheDocument();
  },

  async claimedDutChipIsVisible(dutId: string) {
    expect(await findChipForDut(dutId)).toBeInTheDocument();
  },

  claimedDutChipIsNotVisible(dutId: string) {
    expect(screen.queryByText(new RegExp(dutId, 'i'))).not.toBeInTheDocument();
  },

  async claimedDutChipIsDisabled(dutId: string) {
    const chip = await findChipForDut(dutId);
    await waitFor(() => {
      expect(chip).toHaveClass('Mui-disabled');
    });
  },

  async claimedDutChipIsEnabled(dutId: string) {
    const chip = await findChipForDut(dutId);
    await waitFor(() => {
      expect(chip).not.toHaveClass('Mui-disabled');
    });
  },

  async claimedDutChipHasDeleteButton(dutId: string) {
    const chip = await findChipForDut(dutId);
    expect(within(chip).getByTestId('CancelIcon')).toBeInTheDocument();
  },

  async claimedDutChipHasNoDeleteButton(dutId: string) {
    const chip = await findChipForDut(dutId);
    expect(within(chip).queryByTestId('CancelIcon')).not.toBeInTheDocument();
  },

  unclaimButtonCountIs(count: number) {
    expect(screen.getAllByTestId('CancelIcon')).toHaveLength(count);
  },

  timeframeFilterIsVisible() {
    expect(screen.getByText('Timeframe:')).toBeInTheDocument();
  },

  timeframeFilterIsNotVisible() {
    expect(screen.queryByText('Timeframe:')).not.toBeInTheDocument();
  },

  async selectedLabIs(lab: RepairLab) {
    expect(await screen.findByTestId('repair-lab-select')).toHaveTextContent(
      `Lab: ${lab}`,
    );
  },

  loadingPlaceholdersAreVisible(expectedSkeletonCount = 5) {
    expect(screen.getAllByTestId('kpi-skeleton')).toHaveLength(
      expectedSkeletonCount,
    );
    expect(screen.getByTestId('workforce-loading-spinner')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  },

  async errorStateIsVisible(expectedMessage: string | RegExp) {
    const alert = await screen.findByTestId('workforce-activity-error');
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(expectedMessage);
    expect(
      await screen.findByTestId('workforce-error-table'),
    ).toBeInTheDocument();
  },

  async emptyTableIsVisible(timeframe: TimeframeOption = '1D') {
    const cell = await screen.findByTestId('workforce-empty-table');
    expect(cell).toHaveTextContent(
      `No workforce activity recorded for the selected timeframe (${timeframe}).`,
    );
  },
};
