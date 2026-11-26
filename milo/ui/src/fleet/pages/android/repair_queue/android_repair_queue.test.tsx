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

import {
  anAndroidPriorityRule,
  anAndroidRepairQueueItem,
} from '@/fleet/testing_tools/builders/android_repair_queue_item_builder';

import { given } from './android_repair_dashboard_given';
import { then } from './android_repair_dashboard_then';
import { when } from './android_repair_dashboard_when';

describe('<AndroidRepairDashboard /> (Repair Queue & Claiming)', () => {
  jest.setTimeout(25000);

  it('renders Android queue columns, in-flight repair health badges (+10% / +5%), and hides Host Group on Pixel', async () => {
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .withHostGroups(['presubmit_critical'])
        .withRunTarget('seahawk')
        .withLab('MTV-1')
        .withHealth(0.82, 0.95)
        .withInRepairHealth(0.1, 0.05)
        .withScore(650)
        .unclaimed(),
      anAndroidRepairQueueItem(2)
        .withDeviceId('mh-host-brk-1')
        .asHost()
        .withHostGroups(['presubmit_critical'])
        .withLab('MTV-2')
        .withHealth(0.4, undefined)
        .withInRepairHealth(0.1, undefined)
        .withScore(300)
        .claimedBy('user@example.com'),
    ]);

    const { unmount } = when.dashboardIsRendered('Android');

    then.dashboardHeadersAreVisible('Android');
    then.tableColumnsAre([
      'Rank',
      'ID',
      'Machine Type',
      'Lab',
      'Host Group',
      'Run Target',
      'State',
      'Assignee',
      'Host Group / Run Target Health',
      'Priority Score',
    ]);
    await then.repairTaskMatches('android-device-101', {
      rank: 1,
      machineType: 'device',
      lab: 'MTV-1',
      hostGroup: 'presubmit_critical',
      runTarget: 'seahawk',
      health: '82% (+10%)/95% (+5%)',
      score: '+650 pts',
      claimable: true,
    });
    await then.repairTaskMatches('mh-host-brk-1', {
      rank: 2,
      machineType: 'host',
      lab: 'MTV-2',
      health: '40% (+10%)/-',
      score: '+300 pts',
      claimed: true,
    });
    await then.repairTaskIsPinnedWithRank('mh-host-brk-1', 2);

    unmount();

    when.dashboardIsRendered('Pixel');
    then.dashboardHeadersAreVisible('Pixel');
    then.tableColumnsAre([
      'Rank',
      'ID',
      'Machine Type',
      'Lab',
      'Run Target',
      'State',
      'Assignee',
      'Host Group / Run Target Health',
      'Priority Score',
    ]);
  });

  it('claims, unclaims, and reassigns tasks end-to-end against fake pRPC backend', async () => {
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .unclaimed(),
      anAndroidRepairQueueItem(2)
        .withDeviceId('android-device-102')
        .claimedBy('user@example.com'),
      anAndroidRepairQueueItem(3)
        .withDeviceId('android-device-103')
        .claimedBy('teammate@google.com'),
    ]);

    when.dashboardIsRendered('Android');

    await when.userClaimsTask('android-device-101');
    await then.repairTaskIsClaimed('android-device-101');

    await when.userUnclaimsTask('android-device-102');
    await then.repairTaskIsUnclaimed('android-device-102');

    await then.repairTaskIsClaimedByOther(
      'android-device-103',
      'teammate@google.com',
    );
    await when.userReassignsTaskFrom(
      'android-device-103',
      'teammate@google.com',
    );
    await then.repairTaskIsClaimed('android-device-103');
  });

  it('optimistically updates claim state in flight and rolls back when ClaimAndroidRepairTask fails', async () => {
    const claimRpc = given.deferredRpc('ClaimAndroidRepairTask');
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .unclaimed(),
    ]);

    when.dashboardIsRendered('Android');
    await when.userClaimsTask('android-device-101');
    await then.repairTaskIsClaimed('android-device-101');

    await claimRpc.rejectWithError(14, 'Backend unavailable');
    await then.repairTaskIsUnclaimed('android-device-101');
  });

  it('displays priority score breakdown popover with matched, negative, and empty rules', async () => {
    given.priorityRules([
      anAndroidPriorityRule('rule-1', 'run_target = "seahawk"', '700'),
      anAndroidPriorityRule('rule-neg', 'lab_name = "MTV-1"', '-100'),
    ]);
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .withRunTarget('seahawk')
        .withLab('MTV-1'),
      anAndroidRepairQueueItem(2)
        .withDeviceId('android-device-102')
        .withRunTarget('mustang')
        .withLab('MTV-2'),
    ]);

    when.dashboardIsRendered('Android');
    await when.userHoversScoreCell('android-device-101');
    await then.scoreBreakdownShowsRule('rule-1', /2 rules matched/i);
    await then.scoreBreakdownShowsRule('rule-neg', /2 rules matched/i);
    await then.scoreBreakdownShowsTotal('= 600 pts');

    await when.userHoversScoreCell('android-device-102');
    await then.scoreBreakdownShowsEmpty();
  });

  it('navigates from Page 1 to Page 2 while preserving objective queue ranks', async () => {
    const items = Array.from({ length: 102 }, (_, idx) =>
      anAndroidRepairQueueItem(idx + 1)
        .withDeviceId(`android-device-${idx + 1}`)
        .withRank(idx + 1),
    );
    given.repairQueue(items);

    when.dashboardIsRendered('Android');
    await then.repairTaskMatches('android-device-1', { rank: 1 });

    await when.userGoesToNextPage();
    await then.repairTaskMatches('android-device-101', { rank: 101 });
    await then.repairTaskMatches('android-device-102', { rank: 102 });
  });

  it('displays empty state when repair queue has no items', async () => {
    given.emptyRepairQueue();

    when.dashboardIsRendered('Android');
    await then.tableIsEmpty();
  });

  it('displays error alerts when ListAndroidRepairQueue or ListAndroidPriorityRules pRPC calls fail', async () => {
    given.repairQueueError(14, 'Android queue unreachable');
    given.priorityRulesError(14, 'Android rules unavailable');

    when.dashboardIsRendered('Android');
    await then.errorAlertIsVisible('Error Loading Android Repair Queue');
    await then.errorAlertIsVisible(/Android rules unavailable/i);
  });
});
