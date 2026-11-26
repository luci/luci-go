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

describe('<AndroidRepairDashboard /> (Priority Rules)', () => {
  jest.setTimeout(25000);

  it('creates, updates filter & weight, and deletes a priority rule end-to-end while recomputing queue scores', async () => {
    given.priorityRules([
      anAndroidPriorityRule('rule-1', 'run_target = "seahawk"', '400'),
    ]);
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .withRunTarget('seahawk')
        .withHostGroups(['presubmit_critical'])
        .withLab('MTV-1'),
    ]);

    when.dashboardIsRendered('Android');
    await then.ruleExists('rule-1');
    await then.repairTaskMatches('android-device-101', { score: '+400 pts' });

    // Edit weight -> Apply -> score recomputed
    await when.userTypesRuleWeight('rule-1', '600');
    await then.ruleRowShowsApplyCancel('rule-1');
    await when.userClicksRuleApply('rule-1');
    await then.ruleHasWeight('rule-1', 600);
    await then.repairTaskMatches('android-device-101', { score: '+600 pts' });

    // Add draft rule -> Apply -> score recomputed
    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();
    await when.userTypesDraftRuleFilter('Lab:MTV-1');
    await when.userTypesDraftRuleWeight('250');
    await when.userClicksDraftRuleApply();
    await then.ruleExists('rule-2');
    await then.repairTaskMatches('android-device-101', { score: '+850 pts' });

    // Delete rule -> confirm -> score recomputed
    await when.userClicksDeleteRule('rule-2');
    await then.ruleRowShowsConfirmDelete('rule-2');
    await when.userConfirmsDeleteRule('rule-2');
    await then.ruleDoesNotExist('rule-2');
    await then.repairTaskMatches('android-device-101', { score: '+600 pts' });

    // Edit filter -> Apply
    await when.userTypesRuleFilter('rule-1', 'Lab:MTV-1');
    await when.userClicksRuleApply('rule-1');
    await then.ruleHasFilterText('rule-1', /MTV-1/i);
  });

  it('creates an effective_host_group_health_pct range rule via RangeFilter popover', async () => {
    given.repairQueue([
      anAndroidRepairQueueItem(1)
        .withDeviceId('android-device-101')
        .withHealth(0.4, 0.8)
        .withInRepairHealth(0.1, undefined),
    ]);

    when.dashboardIsRendered('Android');
    await when.userClicksAddRule();
    await then.draftRuleRowIsVisible();
    await when.userSelectsDraftRangeFilter('Host Group Health', {
      min: '20',
      max: '60',
    });
    await when.userTypesDraftRuleWeight('300');
    await when.userClicksDraftRuleApply();

    await then.ruleExists('rule-1');
    then.rpcWasCalledWith('CreateAndroidPriorityRule', {
      priorityRule: expect.objectContaining({
        expressionAip160:
          'effective_host_group_health_pct >= 0.2 AND effective_host_group_health_pct <= 0.6',
        weight: '300',
      }),
    });
    await then.repairTaskMatches('android-device-101', { score: '+300 pts' });
  });

  it('supports cancelling draft rule creation, weight edits, and inline delete without mutating backend', async () => {
    given.priorityRules([
      anAndroidPriorityRule('rule-1', 'run_target = "seahawk"', '400'),
    ]);
    given.repairQueue([
      anAndroidRepairQueueItem(1).withDeviceId('android-device-101'),
    ]);

    when.dashboardIsRendered('Android');
    await then.ruleExists('rule-1');

    // Cancel draft creation
    await when.userClicksAddRule();
    await when.userTypesDraftRuleFilter('Lab:MTV-1');
    await when.userClicksDraftRuleCancel();
    then.draftRuleRowIsNotVisible();
    then.rpcWasNotCalled('CreateAndroidPriorityRule');

    // Cancel weight edit
    await when.userTypesRuleWeight('rule-1', '999');
    await when.userClicksRuleCancel('rule-1');
    await then.ruleHasWeight('rule-1', 400);
    then.rpcWasNotCalled('UpdateAndroidPriorityRule');

    // Cancel inline delete
    await when.userClicksDeleteRule('rule-1');
    await when.userCancelsDeleteRule('rule-1');
    await then.ruleRowShowsDeleteIcon('rule-1');
    then.rpcWasNotCalled('DeleteAndroidPriorityRule');
  });

  it('rejects empty filter or out-of-range weight without calling CreateAndroidPriorityRule', async () => {
    given.repairQueue([
      anAndroidRepairQueueItem(1).withDeviceId('android-device-101'),
    ]);

    when.dashboardIsRendered('Android');
    await when.userClicksAddRule();
    await when.userClicksDraftRuleApply();
    await then.validationErrorIsVisible('Filter expression cannot be empty');
    then.rpcWasNotCalled('CreateAndroidPriorityRule');

    await when.userTypesDraftRuleFilter('Lab:MTV-1');
    await when.userTypesDraftRuleWeight('2000000');
    await when.userClicksDraftRuleApply();
    await then.validationErrorIsVisible(
      'Weight must be a valid integer between -1,000,000 and 1,000,000',
    );
    then.rpcWasNotCalled('CreateAndroidPriorityRule');
  });

  it('enforces the 15-rule limit and expands/collapses rules beyond default visible count', async () => {
    given.priorityRules(
      Array.from({ length: 15 }, (_, idx) =>
        anAndroidPriorityRule(`rule-${idx + 1}`, 'run_target = "seahawk"', 100),
      ),
    );
    given.repairQueue([
      anAndroidRepairQueueItem(1).withDeviceId('android-device-101'),
    ]);

    when.dashboardIsRendered('Android');
    await then.ruleExists('rule-1');

    await when.userExpandsAllRules();
    await then.ruleExists('rule-15');
    await then.addRuleButtonIsDisabledWithLimit(15);

    await when.userCollapsesRules();
    await then.ruleDoesNotExist('rule-15');
  });

  it('hides edit controls when user lacks edit permission', async () => {
    given.userCanEditPriorityRules(false);
    given.priorityRules([
      anAndroidPriorityRule('rule-1', 'run_target = "seahawk"', 100),
    ]);
    given.repairQueue([
      anAndroidRepairQueueItem(1).withDeviceId('android-device-101'),
    ]);

    when.dashboardIsRendered('Android');
    await then.ruleEditingIsDisabled('rule-1');
  });

  it('displays error alerts and rolls back optimistic state when rule mutation fails', async () => {
    const deleteRpc = given.deferredRpc('DeleteAndroidPriorityRule');
    given.priorityRules([
      anAndroidPriorityRule('rule-1', 'run_target = "seahawk"', '400'),
    ]);
    given.repairQueue([
      anAndroidRepairQueueItem(1).withDeviceId('android-device-101'),
    ]);

    when.dashboardIsRendered('Android');
    await then.ruleExists('rule-1');

    await when.userClicksDeleteRule('rule-1');
    await when.userConfirmsDeleteRule('rule-1');
    await then.ruleDoesNotExist('rule-1');

    await deleteRpc.rejectWithError(14, 'Rule service unavailable');
    await then.ruleExists('rule-1');
    await then.errorAlertIsVisible(/Rule service unavailable/i);
  });
});
