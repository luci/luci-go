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
  genericThen,
} from '@/fleet/testing_tools/integration/fleet_integration_test';

export const then = {
  ...genericThen,

  filterChipIsVisible: fleetFilterBar.expectChipVisible,

  async overviewDashboardIsVisible() {
    await genericThen.textIsVisible('ChromeOS Fleet Health Metrics');
    await genericThen.textIsVisible(/Analyze hardware reliability trends/i);
    await genericThen.textIsVisible('availability and health');
    expect(
      screen.getByPlaceholderText('Add a filter (e.g. model:volteer)'),
    ).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /^models$/i })).toBeInTheDocument();
    expect(
      await screen.findByText('Active Support Risk Incidents'),
    ).toBeInTheDocument();
    expect(
      await screen.findByTestId('health-slice-item-all-fleet'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('health-page-help-icon')).toBeInTheDocument();
    genericThen.textIsNotVisible('Global Default Expected Quota');
  },

  async configurationButtonIsVisible() {
    expect(
      await screen.findByRole('button', { name: /^configuration$/i }),
    ).toBeInTheDocument();
  },

  async configurationButtonIsNotVisible() {
    await genericThen.waitUntilRemoved(() =>
      screen.queryByRole('button', { name: /^configuration$/i }),
    );
  },

  async sliceBaselineChipIs(sliceId: string, expectedText: string) {
    const chip = await screen.findByTestId(`slice-baseline-chip-${sliceId}`);
    await waitFor(() => {
      expect(chip).toHaveTextContent(expectedText);
    });
  },

  async rankingRowHasDeviceListFilter(
    name: string,
    expectedFilterParam: string,
  ) {
    const row = await screen.findByTestId(`ranking-row-${name}`);
    const devicesLink = within(row).getByRole('link', {
      name: /^view devices$/i,
    });
    const url = new URL(
      devicesLink.getAttribute('href') ?? '',
      'http://localhost',
    );
    expect(url.searchParams.get('filters')).toBe(expectedFilterParam);
  },

  setOverrideButtonIsDisabled() {
    expect(
      screen.getByRole('button', { name: /set override/i }),
    ).toBeDisabled();
  },

  setOverrideButtonIsEnabled() {
    expect(screen.getByRole('button', { name: /set override/i })).toBeEnabled();
  },
};
