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

import { screen, within } from '@testing-library/react';

import { RepairLab } from '@/fleet/pages/chromeos/repairs/use_repair_lab';
import {
  genericWhen,
  getUser,
} from '@/fleet/testing_tools/integration/fleet_integration_test';

import {
  Component as WorkforceActivityRouteComponent,
  TimeframeOption,
  WorkforceActivityView,
  WorkforceActivityViewProps,
} from './workforce_activity_view';

export const when = {
  ...genericWhen,

  viewIsRendered(props: WorkforceActivityViewProps = {}) {
    return genericWhen.renderPage(<WorkforceActivityView {...props} />);
  },

  routeComponentIsRendered() {
    return genericWhen.renderPage(<WorkforceActivityRouteComponent />);
  },

  async userSelectsTimeframe(timeframe: TimeframeOption) {
    const labelMap: Record<TimeframeOption, RegExp> = {
      '1D': /Last Day \(1D\)/i,
      '7D': /Last Week \(7D\)/i,
      '30D': /Last Month \(30D\)/i,
      YTD: /^YTD$/i,
    };
    await genericWhen.userClicksButton(labelMap[timeframe]);
  },

  async userUnclaimsDut(dutId: string) {
    const chipLabel = await screen.findByText(new RegExp(dutId, 'i'));
    const chip = chipLabel.closest<HTMLElement>('.MuiChip-root');
    if (!chip) {
      throw new Error(`Could not locate MUI Chip for DUT ${dutId}`);
    }
    await getUser().click(within(chip).getByTestId('CancelIcon'));
  },

  async userExpandsTechnicianDuts(hiddenCount: number) {
    await genericWhen.userClicksButton(`And ${hiddenCount} more...`);
  },

  async userCollapsesTechnicianDuts() {
    await genericWhen.userClicksButton('Show less');
  },

  async userClicksBackToQueue() {
    await genericWhen.userClicksButton(/Prioritized Manual Repair Queue/i);
  },

  async userSelectsLab(lab: RepairLab) {
    const user = getUser();
    await user.click(await screen.findByTestId('repair-lab-select'));
    await user.click(await screen.findByTestId(`repair-lab-option-${lab}`));
  },
};
