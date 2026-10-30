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
  aPriorityRule,
  aRepairQueueItem,
} from '@/fleet/testing_tools/builders/repair_queue_item_builder';

import { given } from './chromeos_repair_dashboard_given';
import { then } from './chromeos_repair_dashboard_then';
import { when } from './chromeos_repair_dashboard_when';

describe('<ChromeOSRepairDashboard /> (Full Page Integration with Fake pRPC Backend)', () => {
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

  it('creates, updates lab filter & weight, and deletes a priority rule end-to-end while recomputing queue scores', async () => {
    given.priorityRules([aPriorityRule('rule-1', 'model = "volteer"', '650')]);
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .withModel('volteer')
        .withPool('DUT_POOL_QUOTA')
        .withScore(650),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    // Step-by-step edit weight -> Apply -> score recomputed
    await when.userTypesRuleWeight('rule-1', '800');
    await then.ruleRowShowsApplyCancel('rule-1');
    await when.userClicksRuleApply('rule-1');
    await then.ruleHasWeight('rule-1', 800);
    await then.repairTaskMatches('chromeos15-row2-rack3-host4', {
      score: '+800 pts',
    });

    // Step-by-step add draft rule -> filter -> weight -> Apply -> score recomputed
    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();
    await when.userTypesDraftRuleFilter('Pool:DUT_POOL_QUOTA');
    await when.userTypesDraftRuleWeight('300');
    await when.userClicksDraftRuleApply();
    await then.ruleExists('rule-2');
    await then.repairTaskMatches('chromeos15-row2-rack3-host4', {
      score: '+1100 pts',
    });

    // Step-by-step delete rule -> confirm -> score recomputed
    await when.userClicksDeleteRule('rule-2');
    await then.ruleRowShowsConfirmDelete('rule-2');
    await when.userConfirmsDeleteRule('rule-2');
    await then.ruleDoesNotExist('rule-2');
    await then.repairTaskMatches('chromeos15-row2-rack3-host4', {
      score: '+800 pts',
    });

    // Step-by-step edit filter -> Apply
    await when.userTypesRuleFilter('rule-1', 'Lab:MTV');
    await when.userClicksRuleApply('rule-1');
    await then.ruleHasFilterText('rule-1', /MTV/i);
  });

  it('supports cancelling draft rule creation, cancelling in-progress rule edits, and cancelling inline delete', async () => {
    given.priorityRules([aPriorityRule('rule-1', 'model = "volteer"', '650')]);
    given.repairQueue([
      aRepairQueueItem(1)
        .withDutId('chromeos15-row2-rack3-host4')
        .withModel('volteer')
        .withScore(650),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    // 1. Cancel draft rule creation
    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();
    await when.userTypesDraftRuleFilter('Pool:DUT_POOL_QUOTA');
    await when.userTypesDraftRuleWeight('250');
    await when.userClicksDraftRuleCancel();
    then.draftRuleRowIsNotVisible();
    then.rpcWasNotCalled('CreatePriorityRule');

    // 2. Cancel in-progress weight edit on existing rule
    await when.userTypesRuleWeight('rule-1', '999');
    await then.ruleRowShowsApplyCancel('rule-1');
    await when.userClicksRuleCancel('rule-1');
    await then.ruleHasWeight('rule-1', 650);
    await then.ruleRowShowsDeleteIcon('rule-1');
    then.rpcWasNotCalled('UpdatePriorityRule');

    // 3. Cancel inline delete confirmation via Cancel button and Escape key
    await when.userClicksDeleteRule('rule-1');
    await then.ruleRowShowsConfirmDelete('rule-1');
    await when.userCancelsDeleteRule('rule-1');
    await then.ruleRowShowsDeleteIcon('rule-1');
    await then.ruleExists('rule-1');
    then.rpcWasNotCalled('DeletePriorityRule');

    await when.userClicksDeleteRule('rule-1');
    await then.ruleRowShowsConfirmDelete('rule-1');
    await when.userPressesEscape();
    await then.ruleRowShowsDeleteIcon('rule-1');

    // 4. Emptying all filter chips morphs to inline Delete and Cancel restores original chip
    await when.userClearsRuleFilter('rule-1');
    await then.ruleRowShowsConfirmDelete('rule-1');
    await when.userClicksRuleCancel('rule-1');
    await then.ruleRowShowsDeleteIcon('rule-1');
    await then.ruleHasFilterText('rule-1', /volteer/i);
    then.rpcWasNotCalled('DeletePriorityRule');
    then.rpcWasNotCalled('UpdatePriorityRule');
  });

  it('rejects creating an empty priority rule or an out-of-range weight without calling CreatePriorityRule pRPC', async () => {
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();

    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();
    await when.userClicksDraftRuleApply();
    await then.validationErrorIsVisible('Filter expression cannot be empty');
    then.rpcWasNotCalled('CreatePriorityRule');

    await when.userTypesDraftRuleFilter('model:volteer');
    await when.userTypesDraftRuleWeight('2000000');
    await when.userClicksDraftRuleApply();
    await then.validationErrorIsVisible(
      'Weight must be a valid integer between -1,000,000 and 1,000,000',
    );
    then.rpcWasNotCalled('CreatePriorityRule');
  });

  it('enforces the 15-rule limit and expands/collapses rules beyond default visible count', async () => {
    given.priorityRules(
      Array.from({ length: 15 }, (_, idx) =>
        aPriorityRule(`rule-${idx + 1}`, 'model = "volteer"', '100'),
      ),
    );
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    await when.userExpandsAllRules();
    await then.ruleExists('rule-15');
    await then.addRuleButtonIsDisabledWithLimit(15);

    await when.userCollapsesRules();
    await then.ruleDoesNotExist('rule-15');
  });

  it('hides edit controls in PriorityRulesPanel when user lacks edit permission', async () => {
    given.userCanEditPriorityRules(false);
    given.priorityRules([aPriorityRule('rule-1', 'model = "volteer"', '650')]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.ruleEditingIsDisabled('rule-1');
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

  it('allows adding an IRM filter (Irm:b/458449804) to priority rules via dynamic device label dimensions', async () => {
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await when.userCreatesRule({
      filterText: 'Irm:b/458449804',
      weight: '900',
    });

    await then.ruleExists('rule-1');
    await then.ruleHasFilterText('rule-1', /458449804/i);
    await then.ruleHasWeight('rule-1', 900);
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

  it('validates empty rows, shows Matches all devices row with disabled add buttons, and saves Match all devices', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
        expressionsAip160: [],
        affectedDevicesCount: 12,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: 'Add filter', affectedCount: 12 },
    );

    // 1. Open dialog -> Add filter row -> click Save without typing -> validation error
    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmAddFilterRow();
    await when.userClicksIrmDialogSave();
    await then.irmFilterValidationErrorIsVisible(
      'Filter expression cannot be empty',
    );
    then.rpcWasNotCalled('UpdateIrmIncident');

    // 2. Click Match all devices -> shows Matches all devices row & disables Add filter / Match all buttons
    await when.userClicksIrmMatchAllDevices();
    await then.irmDialogShowsMatchAllRow();

    // 3. Delete Match all devices row -> re-enables Add filter / Match all buttons
    await when.userClicksIrmDeleteRow(0);
    await then.irmDialogAddButtonsAreEnabled();

    // 4. Re-select Match all devices -> Save -> persists via UpdateIrmIncident
    await when.userClicksIrmMatchAllDevices();
    await when.userClicksIrmDialogSave();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: 'All devices', affectedCount: 12 },
    );
    then.rpcWasCalledWith('UpdateIrmIncident', {
      id: 'irm-1',
      expressionsAip160: [''],
    });
  });

  it('displays tooltip expressions on IRM filter badge and discards changes on Cancel', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
        expressionsAip160: ['model = "brya"', 'pools = "DUT_POOL_QUOTA"'],
        affectedDevicesCount: 12,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: '2 applied', affectedCount: 12 },
    );

    // 1. Hovering filter badge shows tooltip with all AIP-160 expressions
    await when.userHoversIrmFilterBadge('458449804');
    await then.irmFilterTooltipContains([
      'model = "brya"',
      'pools = "DUT_POOL_QUOTA"',
    ]);

    // 2. Open dialog -> delete filter row -> click Cancel -> discards changes without pRPC call
    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmDeleteRow(0);
    await when.userClicksIrmDialogCancel();
    then.irmDialogIsClosed();
    then.rpcWasNotCalled('UpdateIrmIncident');
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: '2 applied', affectedCount: 12 },
    );
  });

  it('deletes one of multiple IRM filter expressions and clears filters when the last is deleted', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
        expressionsAip160: ['model = "brya"', 'pools = "DUT_POOL_QUOTA"'],
        affectedDevicesCount: 12,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: '2 applied', affectedCount: 12 },
    );

    // 1. Delete one of two expressions -> Save -> preserves remaining expression
    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmDeleteRow(0);
    await when.userClicksIrmDialogSave();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: '1 applied', affectedCount: 12 },
    );
    then.rpcWasCalledWith('UpdateIrmIncident', {
      id: 'irm-1',
      expressionsAip160: ['pools = "DUT_POOL_QUOTA"'],
    });

    // 2. Delete final remaining expression -> Save -> clears filters back to 'Add filter'
    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmDeleteRow(0);
    await when.userClicksIrmDialogSave();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: 'Add filter', affectedCount: 12 },
    );
    then.rpcWasCalledWith('UpdateIrmIncident', {
      id: 'irm-1',
    });
  });

  it('adds a scoped AIP-160 filter expression in IrmFilterDialog and persists via UpdateIrmIncident', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
        expressionsAip160: [],
        affectedDevicesCount: 12,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: 'Add filter', affectedCount: 12 },
    );

    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmAddFilterRow();
    await when.userTypesIrmFilterExpression('model:volteer');
    await when.userClicksIrmDialogSave();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: '1 applied', affectedCount: 12 },
    );
    then.rpcWasCalledWith('UpdateIrmIncident', {
      id: 'irm-1',
      expressionsAip160: ['(model = "volteer")'],
    });
  });

  it('enforces read-only mode on ActiveIrmTable and IrmFilterDialog when user lacks edit permission', async () => {
    given.userCanEditPriorityRules(false);
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Unfiltered outage',
        expressionsAip160: [],
        affectedDevicesCount: 4,
      },
      {
        id: 'irm-2',
        masterBugId: '542600013',
        title: 'Filtered outage',
        expressionsAip160: ['model = "volteer"'],
        affectedDevicesCount: 7,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentHasNoFiltersReadOnly('458449804');

    await when.userOpensIrmFilterDialog('542600013');
    await then.irmFilterDialogIsReadOnly();
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

  it('creates a pool health range filter rule via RangeFilter popover', async () => {
    given.priorityRules([
      aPriorityRule('rule-1', 'model = "volteer"', '650'),
      aPriorityRule('rule-2', 'model = "brya"', '500'),
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    await when.userClicksAddRule();
    await when.userSelectsDraftRangeFilter('Pool Health', { max: '80' });
    await when.userTypesDraftRuleWeight('400');
    await when.userClicksDraftRuleApply();
    await then.ruleExists('rule-3');
    await then.ruleHasFilterText('rule-3', /Pool Health <= 80%/i);
    await then.ruleHasWeight('rule-3', 400);
  });

  it('submits rule weight on Enter and preserves dirty edits and draft rows on other rules', async () => {
    given.priorityRules([
      aPriorityRule('rule-1', 'model = "volteer"', '650'),
      aPriorityRule('rule-2', 'model = "brya"', '500'),
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    // Dirty edit on rule-2 + open draft row survive when rule-1 is submitted via Enter
    await when.userTypesRuleWeight('rule-2', '999');
    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();

    await when.userSubmitsRuleWeightWithEnter('rule-1', '750');
    await then.ruleHasWeight('rule-1', 750);
    await then.ruleHasWeightInputValue('rule-2', 999);
    await then.ruleRowShowsApplyCancel('rule-2');
    await then.draftRuleRowIsVisible();
  });

  it('displays error alerts and rolls back optimistic state when PriorityRule mutations fail', async () => {
    given.priorityRules([aPriorityRule('rule-1', 'model = "volteer"', '650')]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.ruleExists('rule-1');

    // 1. CreatePriorityRule failure shows server error message
    given.rpcError('CreatePriorityRule', 3, 'Invalid AIP-160 rule syntax');
    await when.userCreatesRule({ filterText: 'model:volteer', weight: '100' });
    await then.validationErrorIsVisible(/Invalid AIP-160 rule syntax/i);
    await when.userClicksDraftRuleCancel();

    // 2. UpdatePriorityRule failure rolls back optimistic weight
    const updateRpc = given.deferredRpc('UpdatePriorityRule');
    await when.userUpdatesRuleWeight('rule-1', '900');
    await then.ruleHasWeightInputValue('rule-1', 900);
    await updateRpc.rejectWithError(14, 'Update rule failed');
    await then.ruleHasWeight('rule-1', 650);

    // 3. DeletePriorityRule failure rolls back optimistic removal
    const deleteRpc = given.deferredRpc('DeletePriorityRule');
    await when.userDeletesRule('rule-1');
    await then.ruleDoesNotExist('rule-1');
    await deleteRpc.rejectWithError(14, 'Delete rule failed');
    await then.ruleExists('rule-1');
  });

  it('displays sync spinner in flight and error alert when UpdateIrmIncident mutation fails', async () => {
    given.irmIncidents([
      {
        id: 'irm-1',
        masterBugId: '458449804',
        title: 'Fleet-wide lab servo outage',
        expressionsAip160: [],
        affectedDevicesCount: 4,
      },
    ]);
    given.repairQueue([
      aRepairQueueItem(1).withDutId('chromeos15-row2-rack3-host4'),
    ]);

    when.dashboardIsRendered();
    await then.irmIncidentIsVisible(
      '458449804',
      'Fleet-wide lab servo outage',
      { filterLabel: 'Add filter', affectedCount: 4 },
    );

    const irmRpc = given.deferredRpc('UpdateIrmIncident');
    await when.userOpensIrmFilterDialog('458449804');
    await when.userClicksIrmMatchAllDevices();
    await when.userClicksIrmDialogSave();
    await then.irmIncidentIsSyncing('458449804');
    await irmRpc.rejectWithError(3, 'Invalid IRM filter expression');
    await then.irmMutationErrorIsVisible(/Invalid IRM filter expression/i);
  });
});
