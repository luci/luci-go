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
  aPriorityRule,
  aRepairQueueItem,
} from '@/fleet/testing_tools/builders/repair_queue_item_builder';

import { given } from './chromeos_repair_dashboard_given';
import { then } from './chromeos_repair_dashboard_then';
import { when } from './chromeos_repair_dashboard_when';

describe('<ChromeOSRepairDashboard /> (Repair Queue & Claiming)', () => {
  jest.setTimeout(25000);

  it('renders page header, priority rules panel, active IRM table, and prioritized queue rows from fake pRPC backend', async () => {
    given.priorityRules([
      aPriorityRule('rule-1', 'model = "volteer"', '650'),
      aPriorityRule('rule-2', 'model = "brya"', '500'),
    ]);
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
      },
      {
        id: 'irm-2',
        masterBugId: '',
        title: '',
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .withPool('DUT_POOL_QUOTA')
        .withModel('volteer')
        .withHealth(0.824, 0.95)
        .withScore(650)
        .withActiveIrmIds(['458449804', '458449804'])
        .unclaimed(),
      aRepairQueueItem(2)
        .withDutId('chromeos15-row2-rack3-host5')
        .withPool('faft-cr50')
        .withModel('brya')
        .withHealth(0.45, 0.7)
        .withScore(500)
        .claimedBy('user@example.com'),
      aRepairQueueItem(3).withDeviceId('').withPool('missing-id-pool'),
    ]);

    when.dashboardIsRendered();

    then.dashboardHeadersAreVisible();
    then.tableColumnsAre([
      'Rank',
      'ID',
      'Pool',
      'Model',
      'State',
      'Active IRM',
      'Assignee',
      'Peripherals',
      'Pool / Model Health',
      'Priority Score',
    ]);
    await then.irmIncidentIsVisible('458449804', 'Fleet-wide lab servo outage');
    await then.irmIncidentWithoutBugFallbackIsVisible();
    await then.repairTaskMatches('chromeos15-row2-rack3-host4', {
      rank: 1,
      taskLink: '/ui/fleet/p/chromeos/devices/chromeos15-row2-rack3-host4',
      pool: 'DUT_POOL_QUOTA',
      model: 'volteer',
      health: '82% / 95%',
      score: '+650 pts',
      claimable: true,
    });
    await then.repairTaskMatches('chromeos15-row2-rack3-host5', {
      rank: 2,
      score: '+500 pts',
      claimed: true,
    });
    await then.repairTaskHasPeripherals('chromeos15-row2-rack3-host4', [
      'Servo: OK',
      'Wi-Fi: BROKEN',
      'Bluetooth: MISSING',
    ]);
    await then.repairTaskHasSingleIrmBadge(
      'chromeos15-row2-rack3-host4',
      '458449804',
    );
    await then.repairTaskHasMissingIdFallback('missing-id-pool');
  });

  it('claims an unclaimed task and unclaims a claimed task end-to-end against fake pRPC backend', async () => {
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4').unclaimed(),
      aRepairQueueItem(2)
        .withDutId('chromeos15-row2-rack3-host5')
        .claimedBy('user@example.com'),
    ]);

    when.dashboardIsRendered();
    await when.userClaimsTask('chromeos15-row2-rack3-host4');
    await then.repairTaskIsClaimed('chromeos15-row2-rack3-host4');

    await when.userUnclaimsTask('chromeos15-row2-rack3-host5');
    await then.repairTaskIsUnclaimed('chromeos15-row2-rack3-host5');
  });

  it('synchronizes authenticated identity between UI and fake pRPC backend when claiming a task', async () => {
    given.authenticatedAs('user:alice@google.com', 'alice@google.com');
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4').unclaimed(),
    ]);

    when.dashboardIsRendered();
    await when.userClaimsTask('chromeos15-row2-rack3-host4');
    await then.repairTaskIsClaimed('chromeos15-row2-rack3-host4');
  });

  it('optimistically updates claim state in flight and rolls back when ClaimRepairTask pRPC call fails', async () => {
    const claimRpc = given.deferredRpc('ClaimRepairTask');
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4').unclaimed(),
    ]);

    when.dashboardIsRendered();
    await when.userClaimsTask('chromeos15-row2-rack3-host4');
    await then.repairTaskIsClaimed('chromeos15-row2-rack3-host4');

    await claimRpc.rejectWithError(14, 'Backend unavailable');
    await then.repairTaskIsUnclaimed('chromeos15-row2-rack3-host4');
  });

  it('displays empty state banners when repair queue and active IRM table have no items', async () => {
    given.emptyRepairQueue();
    given.emptyIrmIncidents();

    when.dashboardIsRendered();
    await then.tableIsEmpty();
    await then.irmTableIsEmpty();
  });

  it('displays error alerts when ListRepairQueue, ListPriorityRules, or ListIrmIncidents pRPC calls fail', async () => {
    given.repairQueueError(14, 'Backend service is currently unreachable');
    given.priorityRulesError(14, 'Priority rules service unavailable');
    given.irmIncidentsError(14, 'IRM service unavailable');

    when.dashboardIsRendered();
    await then.repairQueueShowsError();
    await then.priorityRulesShowsError();
    await then.irmTableShowsError();
  });

  it('navigates from Page 1 to Page 2 via user pagination action and displays global ranks (51, 52)', async () => {
    given.initialUrl('/?limit=50');
    given.repairQueue(
      Array.from({ length: 52 }, (_, idx) =>
        aRepairQueueItem(idx + 1).withDutId(
          `chromeos15-row2-rack3-host${idx + 1}`,
        ),
      ),
    );

    when.dashboardIsRendered();
    await then.repairTaskHasRank('chromeos15-row2-rack3-host1', 1);

    await when.userGoesToNextPage();
    await then.repairTaskHasRank('chromeos15-row2-rack3-host51', 51);
    await then.repairTaskHasRank('chromeos15-row2-rack3-host52', 52);
  });

  it('renders Workforce Activity button with In-Progress chip and navigates when feature flag is enabled', async () => {
    given.workforceActivityEnabled();
    given.repairQueue([
      aRepairQueueItem(1).unclaimed(),
      aRepairQueueItem(2).claimedBy('user@example.com'),
    ]);

    when.dashboardIsRendered();
    await then.workforceActivityShowsInProgress(1);

    await when.userOpensWorkforceActivity();
    then.navigatedTo('/ui/fleet/p/chromeos/repairs/workforce');
  });

  it('hides Workforce Activity button when feature flag is disabled', () => {
    given.workforceActivityDisabled();
    given.repairQueue([aRepairQueueItem(1).unclaimed()]);

    when.dashboardIsRendered();
    then.workforceActivityIsNotVisible();
  });

  it('automatically polls backend every 10s (refetchInterval) and reflects updated repair queue & IRM incidents', async () => {
    jest.useFakeTimers();
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Initial lab servo incident',
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4').unclaimed(),
    ]);

    when.dashboardIsRendered();
    await then.repairTaskIsUnclaimed('chromeos15-row2-rack3-host4');
    await then.irmIncidentIsVisible('458449804', 'Initial lab servo incident');

    // Simulate backend state changes while the user has the dashboard open.
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .claimedBy('other-engineer@google.com'),
    ]);
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Initial lab servo incident',
      },
      {
        id: 'irm-2',
        masterBugId: '542600013',
        title: 'Newly opened chameleon outage',
      },
    ]);

    // Advance TanStack Query's 10s refetchInterval timer without any manual rerender.
    await when.timePasses(10_000);

    await then.repairTaskIsClaimedByOther(
      'chromeos15-row2-rack3-host4',
      'other-engineer@google.com',
    );
    await then.irmIncidentIsVisible(
      '542600013',
      'Newly opened chameleon outage',
    );
  });

  it('allows taking over a repair task claimed by another engineer without incrementing In-Progress', async () => {
    given.workforceActivityEnabled();
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .claimedBy('other-engineer@google.com'),
    ]);

    when.dashboardIsRendered();
    await then.workforceActivityShowsInProgress(1);
    await then.repairTaskIsClaimedByOther(
      'chromeos15-row2-rack3-host4',
      'other-engineer@google.com',
    );

    await when.userReassignsTaskFrom(
      'chromeos15-row2-rack3-host4',
      'other-engineer@google.com',
    );
    await then.repairTaskIsClaimed('chromeos15-row2-rack3-host4');
    await then.workforceActivityShowsInProgress(1);
  });

  it('displays priority score breakdown popover with matched, negative, unknown, and empty rules', async () => {
    given.priorityRules([
      aPriorityRule('rule-1', 'model = "volteer"', '650'),
      aPriorityRule('rule-neg', 'pool = "faft-cr50"', '-50'),
    ]);
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .withModel('volteer')
        .withPool('faft-cr50'),
      aRepairQueueItem(2)
        .withDutId('chromeos15-row2-rack3-host5')
        .withModel('brya')
        .withPool('DUT_POOL_QUOTA'),
    ]);

    when.dashboardIsRendered();
    await when.userHoversScoreCell('chromeos15-row2-rack3-host4');
    await then.scoreBreakdownShowsRule('rule-1', /2 rules matched/i);
    await then.scoreBreakdownShowsRule('rule-neg', /2 rules matched/i);
    await then.scoreBreakdownShowsTotal('= 600 pts');

    await when.userHoversScoreCell('chromeos15-row2-rack3-host5');
    await then.scoreBreakdownShowsEmpty();

    // Deleting a rule removes it from rulesById immediately; while the queue
    // refetch is still in flight, hovering falls back to `Rule #<id>`.
    const pendingQueueRefetch = given.deferredRpc('ListRepairQueue');
    await when.userDeletesRule('rule-neg');
    await when.userHoversScoreCell('chromeos15-row2-rack3-host4');
    await then.scoreBreakdownShowsUnknownRule('rule-neg');
    await pendingQueueRefetch.resolveWith({
      repairQueueItems: [],
      totalSize: 0,
      nextPageToken: '',
    });
  });

  it('restores lab from localStorage, switches active lab via LabSelector, syncs URL & storage, and clears cursor', async () => {
    given.initialUrl('/?cursor=page2token');
    given.storedRepairLab('IAD65');
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.selectedLabIs('IAD65');

    await when.userSelectsLab('EM25');
    await then.selectedLabIs('EM25');
    then.searchParamIs('lab', 'EM25');
    then.searchParamIs('cursor', null);
    then.storedRepairLabIs('EM25');
    then.rpcWasCalledWith('ListRepairQueue', { lab: 'EM25' });
  });

  it('renders in-flight repair health badges (+10% / +5%) and Active IRM bug chips in queue rows', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .withHealth(0.82, 0.95)
        .withInRepairHealth(0.1, 0.05)
        .withActiveIrmIds(['458449804']),
    ]);

    when.dashboardIsRendered();
    await then.repairTaskMatches('chromeos15-row2-rack3-host4', {
      rank: 1,
      health: '82% (+10%)/95% (+5%)',
      activeIrmBugId: '458449804',
    });
  });
});
