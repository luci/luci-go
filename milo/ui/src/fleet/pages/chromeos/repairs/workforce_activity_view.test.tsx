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

/* eslint-disable jest/expect-expect */

import {
  aTechnicianActivity,
  buildWorkforceActivityJson,
  given,
} from './workforce_activity_given';
import { then } from './workforce_activity_then';
import { when } from './workforce_activity_when';

describe('<WorkforceActivityView /> (Full Page Integration with Fake pRPC Backend)', () => {
  it('renders header, KPI summary cards, and technician rows from fake pRPC backend', async () => {
    given.allMetricFlagsEnabled();
    given.workforceActivity({
      activeRepairers: 4,
      totalPriorityPointsCleared: 2850,
      avgQueuePickupRank: 4.7,
      inProgressRepairs: 2,
      avgRepairTime: { seconds: '1680', nanos: 0 },
      technicians: [
        aTechnicianActivity({
          id: 'andrew',
          name: 'Andrew Miller',
          email: 'andrew@google.com',
          claimedDuts: [
            {
              taskId: '201',
              dutId: 'chromeos1-row2-rack3-host1',
              score: 250,
              duration: { seconds: '2700', nanos: 0 },
            },
          ],
          priorityScoreCleared: 680,
          avgPtsPerDut: 85,
          avgQueuePickupRank: 1.8,
          isRankSkewed: false,
          completedRepairs: 8,
          avgDuration: { seconds: '1920', nanos: 0 },
        }),
        aTechnicianActivity({
          id: 'beatrice',
          name: 'Beatrice Chen',
          email: 'beatrice@google.com',
          claimedDuts: [
            {
              taskId: '202',
              dutId: 'chromeos2-row3-rack1-host2',
              score: 85,
              duration: { seconds: '1800', nanos: 0 },
            },
          ],
          priorityScoreCleared: 940,
          avgPtsPerDut: 85,
          avgQueuePickupRank: 1.2,
          isRankSkewed: true,
          completedRepairs: 11,
          avgDuration: { seconds: '1440', nanos: 0 },
        }),
        aTechnicianActivity({
          id: 'charlie',
          name: '',
          email: 'user:charlie@google.com',
          claimedDuts: [],
          priorityScoreCleared: 0,
          avgPtsPerDut: 0,
          avgQueuePickupRank: 0,
          completedRepairs: 0,
          avgDuration: undefined,
        }),
      ],
    });

    when.viewIsRendered();

    then.headerIsVisible();
    await then.kpiCardIsVisible('Active Repairers', '4');
    await then.kpiCardIsVisible('Total Priority Points Cleared', '2,850 pts');
    await then.kpiCardIsVisible('Avg Queue Pickup Rank', '#4.7');
    await then.kpiCardIsVisible('In-Progress Repairs', '2');
    await then.kpiCardIsVisible('Avg Repair Time (MTTR)', '28 min');

    await then.technicianRowMatches('Andrew Miller', {
      email: 'andrew@google.com',
      scoreCleared: '680 pts',
      avgPtsPerDut: 'avg 85 pts/DUT',
      pickupRank: '#1.8',
      completedRepairs: '8 devices',
      mttr: '32 min',
    });
    await then.technicianRowMatches('Beatrice Chen', {
      email: 'beatrice@google.com',
      scoreCleared: '940 pts',
      avgPtsPerDut: 'avg 85 pts/DUT',
      pickupRank: '#1.2',
      completedRepairs: '11 devices',
      mttr: '24 min',
    });
    await then.technicianRowMatches('user:charlie@google.com', {
      noClaimedDuts: true,
      scoreCleared: '0 pts',
      completedRepairs: '0 devices',
      mttr: '-',
    });
    then.textIsNotVisible(/Sum:/i);
  });

  it('hides optional metric cards, table columns, and timeframe filter when flags are disabled', async () => {
    given.pickupRankEnabled(false);
    given.inProgressRepairsEnabled(false);
    given.timeframeFilterEnabled(false);
    given.workforceActivity({
      activeRepairers: 2,
      totalPriorityPointsCleared: 500,
      avgQueuePickupRank: 2.5,
      inProgressRepairs: 3,
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.viewIsRendered();

    await then.technicianIsVisible('Andrew Miller');
    await then.kpiCardIsVisible('Active Repairers', '2');
    await then.kpiCardIsVisible('Total Priority Points Cleared', '500 pts');
    then.kpiCardIsNotVisible('Avg Queue Pickup Rank');
    then.kpiCardIsNotVisible('In-Progress Repairs');
    then.timeframeFilterIsNotVisible();
    then.textIsNotVisible('Currently Claimed DUTs (In-Progress)');
  });

  it('unclaims claimed DUTs for any technician row via UnclaimRepairTask pRPC', async () => {
    given.inProgressRepairsEnabled();
    given.authenticatedAs('user:Andrew@Google.com', 'Andrew@Google.com');
    given.workforceActivity({
      technicians: [
        aTechnicianActivity({
          id: 'andrew',
          name: 'Andrew Miller',
          claimedDuts: [
            {
              taskId: '201',
              dutId: 'chromeos1-row2-rack3-host1',
              score: 250,
              duration: { seconds: '2700', nanos: 0 },
            },
          ],
        }),
        aTechnicianActivity({
          id: 'beatrice',
          name: 'Beatrice Chen',
          email: 'beatrice@google.com',
          claimedDuts: [
            {
              taskId: '202',
              dutId: 'chromeos2-row3-rack1-host2',
              score: 85,
              duration: { seconds: '1800', nanos: 0 },
            },
          ],
        }),
      ],
    });

    when.viewIsRendered();

    await then.claimedDutChipIsVisible('chromeos1-row2-rack3-host1');
    await then.claimedDutChipIsVisible('chromeos2-row3-rack1-host2');
    then.unclaimButtonCountIs(2);

    await when.userUnclaimsDut('chromeos1-row2-rack3-host1');
    then.rpcWasCalledWith('UnclaimRepairTask', { taskId: '201' });

    await when.userUnclaimsDut('chromeos2-row3-rack1-host2');
    then.rpcWasCalledWith('UnclaimRepairTask', { taskId: '202' });
  });

  it('formats claimed DUT durations using at most two highest time units', async () => {
    given.inProgressRepairsEnabled();
    given.workforceActivity({
      technicians: [
        aTechnicianActivity({
          claimedDuts: [
            {
              taskId: '301',
              dutId: 'dut-multi-units',
              score: 120,
              // 1 day, 2 hr, 15 min, 30 sec = 94530s
              duration: { seconds: '94530', nanos: 0 },
            },
            {
              taskId: '302',
              dutId: 'dut-three-units',
              score: 90,
              // 1 hr, 2 min, 30 sec = 3750s
              duration: { seconds: '3750', nanos: 0 },
            },
          ],
        }),
      ],
    });

    when.viewIsRendered();

    await then.textIsVisible('dut-multi-units (Score: 120 | 1 day, 2 hr)');
    await then.textIsVisible('dut-three-units (Score: 90 | 1 hr, 2 min)');
  });

  it('shows at most 2 claimed DUTs initially, expands on "And x more...", and collapses on "Show less"', async () => {
    given.inProgressRepairsEnabled();
    given.workforceActivity({
      technicians: [
        aTechnicianActivity({
          claimedDuts: Array.from({ length: 4 }, (_, i) => ({
            taskId: `40${i + 1}`,
            dutId: `chromeos-dut-${i + 1}`,
            score: 100,
            duration: { seconds: '1800', nanos: 0 },
          })),
        }),
      ],
    });

    when.viewIsRendered();

    await then.claimedDutChipIsVisible('chromeos-dut-1\\b');
    await then.claimedDutChipIsVisible('chromeos-dut-2\\b');
    then.claimedDutChipIsNotVisible('chromeos-dut-3');
    then.claimedDutChipIsNotVisible('chromeos-dut-4');

    await when.userExpandsTechnicianDuts(2);
    await then.claimedDutChipIsVisible('chromeos-dut-3');
    await then.claimedDutChipIsVisible('chromeos-dut-4');
    then.textIsNotVisible(/And \d+ more\.\.\./i);

    await when.userCollapsesTechnicianDuts();
    then.claimedDutChipIsNotVisible('chromeos-dut-3');
    then.claimedDutChipIsNotVisible('chromeos-dut-4');
    await then.textIsVisible('And 2 more...');
  });

  it('disables claimed DUT chips while unclaim is in flight and omits delete button when taskId is empty or 0', async () => {
    given.inProgressRepairsEnabled();
    given.authenticatedAs('user:andrew@google.com', 'andrew@google.com');
    const unclaimRpc = given.deferredUnclaim();
    given.workforceActivity({
      technicians: [
        aTechnicianActivity({
          claimedDuts: [
            {
              taskId: '201',
              dutId: 'dut-valid-task',
              score: 150,
              duration: { seconds: '900', nanos: 0 },
            },
            {
              taskId: '0',
              dutId: 'dut-zero-task',
              score: 100,
              duration: { seconds: '600', nanos: 0 },
            },
            {
              taskId: '',
              dutId: 'dut-empty-task',
              score: 100,
              duration: { seconds: '600', nanos: 0 },
            },
          ],
        }),
      ],
    });

    when.viewIsRendered();
    await when.userExpandsTechnicianDuts(1);

    await then.claimedDutChipHasDeleteButton('dut-valid-task');
    await then.claimedDutChipHasNoDeleteButton('dut-zero-task');
    await then.claimedDutChipHasNoDeleteButton('dut-empty-task');
    await then.claimedDutChipIsEnabled('dut-valid-task');
    await then.claimedDutChipIsEnabled('dut-zero-task');

    await when.userUnclaimsDut('dut-valid-task');

    await then.claimedDutChipIsDisabled('dut-valid-task');
    await then.claimedDutChipIsDisabled('dut-zero-task');
    await then.claimedDutChipIsDisabled('dut-empty-task');

    await unclaimRpc.resolveWith({});

    await then.claimedDutChipIsEnabled('dut-valid-task');
    await then.claimedDutChipIsEnabled('dut-zero-task');
  });

  it('requests updated timeframe from GetWorkforceActivity when toggle buttons are clicked', async () => {
    given.timeframeFilterEnabled();
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.viewIsRendered();

    await then.technicianIsVisible('Andrew Miller');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_1D',
      lab: 'MTV',
    });

    await when.userSelectsTimeframe('7D');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_7D',
      lab: 'MTV',
    });

    await when.userSelectsTimeframe('30D');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_30D',
      lab: 'MTV',
    });

    await when.userSelectsTimeframe('YTD');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_YTD',
      lab: 'MTV',
    });
  });

  it('forwards lab prop to GetWorkforceActivity RPC and LabSelector', async () => {
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.viewIsRendered({ lab: 'EM25' });

    await then.selectedLabIs('EM25');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_1D',
      lab: 'EM25',
    });
  });

  it('restores lab from localStorage and updates GetWorkforceActivity requests when LabSelector changes', async () => {
    given.storedRepairLab('EM25');
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.viewIsRendered();

    await then.selectedLabIs('EM25');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_1D',
      lab: 'EM25',
    });

    await when.userSelectsLab('IAD65');
    await then.selectedLabIs('IAD65');
    then.rpcWasCalledWith('GetWorkforceActivity', {
      timeframe: 'TIMEFRAME_1D',
      lab: 'IAD65',
    });
  });

  it('navigates back to the repair queue on back button click', async () => {
    given.mountedPath('/p/:platform/repairs/workforce');
    given.initialUrl('/p/chromeos/repairs/workforce');
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.viewIsRendered();
    await when.userClicksBackToQueue();

    then.navigatedTo('/ui/fleet/p/chromeos/repairs');
    then.searchParamIs('lab', 'MTV');
  });

  it('calls onBackToQueue prop without navigating when supplied', async () => {
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });
    const mockBack = jest.fn();

    when.viewIsRendered({ onBackToQueue: mockBack });
    await when.userClicksBackToQueue();

    expect(mockBack).toHaveBeenCalledTimes(1);
    then.navigatedTo('/');
  });

  it('renders skeleton placeholders in KPI cards and spinner in table while GetWorkforceActivity is loading', async () => {
    given.allMetricFlagsEnabled();
    const workforceRpc = given.deferredWorkforceActivity();

    when.viewIsRendered();

    then.loadingPlaceholdersAreVisible(5);
    then.technicianIsNotVisible('Andrew Miller');

    await workforceRpc.resolveWith(
      buildWorkforceActivityJson({
        activeRepairers: 1,
        technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
      }),
    );

    await then.technicianIsVisible('Andrew Miller');
  });

  it('renders error alert and error table row when GetWorkforceActivity RPC fails', async () => {
    given.workforceActivityError(7, 'Permission denied');

    when.viewIsRendered();

    await then.errorStateIsVisible(
      /You don't have permission to loading workforce activity data/i,
    );
  });

  it('renders empty table message when no technicians are returned', async () => {
    given.emptyWorkforceActivity();

    when.viewIsRendered();

    await then.emptyTableIsVisible('1D');
  });

  it('renders page Component wrapper when feature flag is enabled', async () => {
    given.workforceActivityEnabled();
    given.workforceActivity({
      technicians: [aTechnicianActivity({ name: 'Andrew Miller' })],
    });

    when.routeComponentIsRendered();

    then.headerIsVisible();
    await then.technicianIsVisible('Andrew Miller');
  });

  it('renders PageNotFoundPage in Component wrapper when feature flag is disabled', async () => {
    given.workforceActivityDisabled();

    when.routeComponentIsRendered();

    await then.textIsVisible('Page not found');
  });
});
