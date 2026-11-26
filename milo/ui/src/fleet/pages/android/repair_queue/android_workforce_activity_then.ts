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

import { genericThen } from '@/fleet/testing_tools/integration/fleet_integration_test';

export const findChipForDevice = async (
  deviceId: string,
): Promise<HTMLElement> => {
  const chipLabel = await screen.findByText(new RegExp(deviceId, 'i'));
  const chip = chipLabel.closest<HTMLElement>('.MuiChip-root');
  if (!chip) {
    throw new Error(`Could not locate MUI Chip for device ${deviceId}`);
  }
  return chip;
};

export const then = {
  ...genericThen,

  headerIsVisible() {
    genericThen.headingIsVisible('Workforce & Technician Monitoring', 4);
  },

  async kpiCardIsVisible(title: string, value: string) {
    const titleEl = await screen.findByText(title);
    const card = titleEl.closest<HTMLElement>('.MuiPaper-root');
    if (!card) {
      throw new Error(`Could not locate KPI card container for "${title}"`);
    }
    expect(await within(card).findByText(value)).toBeInTheDocument();
  },

  async technicianRowMatches(
    name: string,
    expected: {
      email?: string;
      scoreCleared?: string;
      avgPtsPerDevice?: string;
      pickupRank?: string;
      completedRepairs?: string;
      mttr?: string;
    },
  ) {
    const anchor = await screen.findByText(name);
    const row = anchor.closest<HTMLTableRowElement>('tr');
    if (!row) {
      throw new Error(`Could not locate table row for "${name}"`);
    }
    const scope = within(row);
    for (const value of Object.values(expected)) {
      if (value !== undefined) {
        expect(scope.getByText(value)).toBeInTheDocument();
      }
    }
  },

  async claimedDeviceChipIsVisible(deviceId: string) {
    const chip = await findChipForDevice(deviceId);
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveClass('MuiChip-filter');
  },

  async claimedDeviceChipIsDisabled(deviceId: string) {
    const chip = await findChipForDevice(deviceId);
    await waitFor(() => {
      expect(chip).toHaveClass('Mui-disabled');
    });
  },

  async claimedDeviceChipIsEnabled(deviceId: string) {
    const chip = await findChipForDevice(deviceId);
    await waitFor(() => {
      expect(chip).not.toHaveClass('Mui-disabled');
    });
  },

  async claimedDeviceChipHasNoDeleteButton(deviceId: string) {
    const chip = await findChipForDevice(deviceId);
    expect(within(chip).queryByTestId('CancelIcon')).not.toBeInTheDocument();
  },

  async errorStateIsVisible(expectedMessage: string | RegExp) {
    const alert = await screen.findByTestId('workforce-activity-error');
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(expectedMessage);
  },

  async unclaimErrorSnackbarIsVisible(expectedMessage: string | RegExp) {
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(expectedMessage);
  },

  async pageNotFoundIsVisible() {
    expect(await screen.findByText('Page not found')).toBeInTheDocument();
  },
};
