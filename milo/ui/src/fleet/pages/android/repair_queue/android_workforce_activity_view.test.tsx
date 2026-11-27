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

import { waitFor } from '@testing-library/react';

import { anAndroidTechnician, given } from './android_workforce_activity_given';
import { then } from './android_workforce_activity_then';
import { when } from './android_workforce_activity_when';

describe('<AndroidWorkforceActivityView /> (Full Page Integration with Fake pRPC Backend)', () => {
  it('requests ANDROID_WORKSPACE_ANDROID and renders Android labels, KPI cards, and technician rows', async () => {
    given.allFlagsEnabled();
    given.workforceActivity({
      activeRepairers: 2,
      totalPriorityPointsCleared: 820,
      avgQueuePickupRank: 1.4,
      inProgressRepairs: 1,
      avgRepairTime: { seconds: '1500', nanos: 0 },
      technicians: [
        anAndroidTechnician({
          id: 'alex',
          name: 'Alex Rivera',
          email: 'alex@example.com',
          claimedDevices: [
            {
              taskId: '501',
              deviceId: 'android-dev-1',
              score: 200,
              duration: { seconds: '900', nanos: 0 },
            },
          ],
          priorityScoreCleared: 500,
          avgPtsPerDevice: 100,
          avgQueuePickupRank: 1.2,
          completedRepairs: 5,
          avgDuration: { seconds: '1200', nanos: 0 },
        }),
      ],
    });

    when.viewIsRendered({ workspace: 'Android' });

    then.headerIsVisible();
    await then.kpiCardIsVisible('Active Repairers', '2');
    await then.kpiCardIsVisible('Total Priority Points Cleared', '820 pts');
    await then.kpiCardIsVisible('Avg Queue Pickup Rank', '#1.4');
    await then.kpiCardIsVisible('In-Progress Repairs', '1');
    await then.kpiCardIsVisible('Avg Repair Time (MTTR)', '25 min');
    await then.textIsVisible('Currently claimed devices');
    await then.textIsVisible('Currently Claimed Devices (In-Progress)');
    await then.technicianRowMatches('Alex Rivera', {
      email: 'alex@example.com',
      scoreCleared: '500 pts',
      avgPtsPerDevice: 'avg 100 pts/device',
      pickupRank: '#1.2',
      completedRepairs: '5 devices',
      mttr: '20 min',
    });
    await then.claimedDeviceChipIsVisible('android-dev-1');
    then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
      workspace: 'ANDROID_WORKSPACE_ANDROID',
      timeframe: 'TIMEFRAME_1D',
    });
  });

  it('requests ANDROID_WORKSPACE_PIXEL for Pixel workspace and preserves workspace across timeframe switches', async () => {
    given.allFlagsEnabled();
    given.workforceActivity({
      activeRepairers: 1,
      technicians: [anAndroidTechnician({ name: 'Jordan Lee' })],
    });

    when.viewIsRendered({ workspace: 'Pixel' });

    await then.textIsVisible('Jordan Lee');
    then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
      workspace: 'ANDROID_WORKSPACE_PIXEL',
      timeframe: 'TIMEFRAME_1D',
    });

    await when.userSelectsTimeframe('7D');
    await waitFor(() => {
      then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
        workspace: 'ANDROID_WORKSPACE_PIXEL',
        timeframe: 'TIMEFRAME_7D',
      });
    });

    await when.userSelectsTimeframe('30D');
    await waitFor(() => {
      then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
        workspace: 'ANDROID_WORKSPACE_PIXEL',
        timeframe: 'TIMEFRAME_30D',
      });
    });

    await when.userSelectsTimeframe('YTD');
    await waitFor(() => {
      then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
        workspace: 'ANDROID_WORKSPACE_PIXEL',
        timeframe: 'TIMEFRAME_YTD',
      });
    });
  });

  it('unclaims an Android task via UnclaimAndroidRepairTask, disables chips while pending, and omits delete when taskId is 0', async () => {
    given.allFlagsEnabled();
    const unclaimRpc = given.deferredUnclaimAndroidTask();
    given.workforceActivity({
      technicians: [
        anAndroidTechnician({
          claimedDevices: [
            {
              taskId: '701',
              deviceId: 'android-claimed-1',
              score: 120,
              duration: { seconds: '600', nanos: 0 },
            },
            {
              taskId: '0',
              deviceId: 'android-unmapped-0',
              score: 40,
              duration: { seconds: '300', nanos: 0 },
            },
          ],
        }),
      ],
    });

    when.viewIsRendered({ workspace: 'Android' });

    await then.claimedDeviceChipIsVisible('android-claimed-1');
    await then.claimedDeviceChipHasNoDeleteButton('android-unmapped-0');

    await when.userUnclaimsDevice('android-claimed-1');
    await then.claimedDeviceChipIsDisabled('android-claimed-1');
    then.rpcWasCalledWith('UnclaimAndroidRepairTask', { taskId: '701' });

    await unclaimRpc.resolveWith({});
    await then.claimedDeviceChipIsEnabled('android-claimed-1');
  });

  it('shows an error Snackbar when UnclaimAndroidRepairTask fails', async () => {
    given.allFlagsEnabled();
    given.unclaimAndroidTaskError(7, 'cannot unclaim task held by another');
    given.workforceActivity({
      technicians: [
        anAndroidTechnician({
          claimedDevices: [
            {
              taskId: '702',
              deviceId: 'android-conflict-1',
              score: 80,
              duration: { seconds: '600', nanos: 0 },
            },
          ],
        }),
      ],
    });

    when.viewIsRendered({ workspace: 'Android' });

    await when.userUnclaimsDevice('android-conflict-1');
    await then.unclaimErrorSnackbarIsVisible(
      /You don't have permission to unclaim repair task/i,
    );
  });

  it('renders error alert when GetAndroidWorkforceActivity fails', async () => {
    given.allFlagsEnabled();
    given.workforceActivityError(13, 'database unavailable');

    when.viewIsRendered({ workspace: 'Android' });

    await then.errorStateIsVisible(/database unavailable/i);
  });

  it('invokes onBackToQueue when Back button is clicked', async () => {
    given.allFlagsEnabled();
    given.workforceActivity({ technicians: [] });
    const onBackToQueue = jest.fn();

    when.viewIsRendered({ workspace: 'Android', onBackToQueue });

    await when.userClicksBackToQueue();
    expect(onBackToQueue).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['Android', '/ui/fleet/p/android/repairs'],
    ['Pixel', '/ui/fleet/p/pixel/repairs'],
  ] as const)(
    'navigates back to the %s repair queue by default when onBackToQueue is omitted',
    async (workspace, expectedPath) => {
      given.allFlagsEnabled();
      given.workforceActivity({ technicians: [] });

      when.viewIsRendered({ workspace });
      await when.userClicksBackToQueue();

      then.navigatedTo(expectedPath);
    },
  );

  it('gates the route wrapper behind enableAndroidRepairQueue', async () => {
    given.allFlagsEnabled();
    given.androidRepairQueueEnabled(false);

    when.routeComponentIsRendered('Android');

    await then.pageNotFoundIsVisible();
  });

  it('gates the route wrapper behind enableAndroidWorkforceActivity', async () => {
    given.allFlagsEnabled();
    given.androidWorkforceActivityEnabled(false);

    when.routeComponentIsRendered('Pixel');

    await then.pageNotFoundIsVisible();
  });

  it('renders the workforce page in route wrapper when both flags are enabled', async () => {
    given.allFlagsEnabled();
    given.workforceActivity({
      activeRepairers: 1,
      technicians: [anAndroidTechnician({ name: 'Sam Taylor' })],
    });

    when.routeComponentIsRendered('Pixel');

    await then.textIsVisible('Sam Taylor');
    then.rpcWasCalledWith('GetAndroidWorkforceActivity', {
      workspace: 'ANDROID_WORKSPACE_PIXEL',
      timeframe: 'TIMEFRAME_1D',
    });
  });
});
