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

/// <reference types="cypress" />

import { mockPrpc } from './common/utils';

describe('ChromeOS Devices Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();

    const dimensionsData = {
      baseDimensions: { machine: { values: ['machine1'] } },
      labels: {
        os: { values: ['ChromeOS'] },
      },
    };
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetDeviceDimensions',
      dimensionsData,
      'getDimensions',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListDevices',
      {
        devices: [{ id: '1', labels: { os: { values: ['ChromeOS'] } } }],
        totalSize: 1,
      },
      'listDevices',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/CountDevices',
      {
        total: 1,
        deviceState: { ready: 1 },
        taskState: { idle: 1, busy: 0 },
      },
      'countDevices',
    );
  });

  it('should load and render table', () => {
    const targetUrl = `/ui/fleet/p/chromeos/devices`;

    cy.visit(targetUrl);

    // Wait for network requests to settle
    cy.wait(['@listDevices', '@countDevices']);

    // Verify table is visible and contains data
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('1').should('be.visible');
  });

  it('should support row selection', () => {
    const targetUrl = `/ui/fleet/p/chromeos/devices`;

    cy.visit(targetUrl);
    cy.wait(['@listDevices', '@countDevices']);

    // Locate the first row checkbox and click it
    cy.get('[data-testid^="select-checkbox-"]').first().click();

    // Verify the row becomes selected/checked
    cy.get('[data-testid^="select-checkbox-"]').first().should('be.checked');
  });
});
