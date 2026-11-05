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

import { aRepairQueueItem } from '@/fleet/testing_tools/builders/repair_queue_item_builder';

import { given } from './chromeos_repair_dashboard_given';
import { then } from './chromeos_repair_dashboard_then';
import { when } from './chromeos_repair_dashboard_when';

describe('<ChromeOSRepairDashboard /> (Active IRM Incidents)', () => {
  jest.setTimeout(25000);

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
