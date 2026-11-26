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

import { within } from '@testing-library/react';

import { TimeframeOption } from '@/fleet/components/repair_queue/workforce_helpers';
import {
  genericWhen,
  getUser,
} from '@/fleet/testing_tools/integration/fleet_integration_test';
import { AndroidPageWorkspace } from '@/fleet/workspaces';

import { findChipForDevice } from './android_workforce_activity_then';
import {
  AndroidWorkforceActivityView,
  AndroidWorkforceActivityViewProps,
  Component as AndroidWorkforceRouteComponent,
} from './android_workforce_activity_view';

export const when = {
  ...genericWhen,

  viewIsRendered(props: AndroidWorkforceActivityViewProps) {
    return genericWhen.renderPage(<AndroidWorkforceActivityView {...props} />);
  },

  routeComponentIsRendered(workspace: AndroidPageWorkspace) {
    return genericWhen.renderPage(
      <AndroidWorkforceRouteComponent workspace={workspace} />,
    );
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

  async userUnclaimsDevice(deviceId: string) {
    const chip = await findChipForDevice(deviceId);
    await getUser().click(within(chip).getByTestId('CancelIcon'));
  },

  async userClicksBackToQueue() {
    await genericWhen.userClicksButton(/Prioritized Manual Repair Queue/i);
  },
};
