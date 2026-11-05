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

describe('<ChromeOSRepairDashboard /> (Priority Rules)', () => {
  jest.setTimeout(25000);

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
});
