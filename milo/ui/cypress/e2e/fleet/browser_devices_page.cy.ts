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

describe('Browser Devices Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();

    const dimensionsData = {
      baseDimensions: { machine: { values: ['machine1'] } },
      swarmingLabels: {
        os: { values: ['Linux', 'Windows'] },
        pool: { values: ['chrome.tests'] },
      },
      ufsLabels: {
        model: { values: ['model1'] },
      },
    };

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetBrowserDeviceDimensions',
      dimensionsData,
      'getDimensions',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListBrowserDevices',
      {
        devices: [{ id: '1', ufsLabels: { os: { values: ['Linux'] } } }],
        totalSize: 1,
      },
      'listDevices',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/CountBrowserDevices',
      {
        total: 1,
        swarmingState: {
          total: 1,
          alive: 1,
          dead: 0,
          quarantined: 0,
          maintenance: 0,
        },
      },
      'countDevices',
    );
  });

  it('should load with filters and render chips', () => {
    const targetUrl = '/ui/fleet/p/chromium/devices';

    cy.visit(targetUrl);

    cy.wait(['@listDevices', '@countDevices']);

    // Type filter in the filter bar after ensuring options are loaded
    cy.get('input[placeholder*="Add a filter"]').click();
    // Wait for options to appear in dropdown
    cy.get('[role="menuitem"]').should('have.length.gt', 0);

    cy.get('[role="menuitem"]').contains('sw.os').click();
    cy.get('[role="menuitem"]').contains('Linux').click();
    cy.contains('button', 'Apply').click();

    // Verify the filter chip is rendered
    cy.contains('[role="button"]', 'os').should('be.visible');
    cy.contains('[role="button"]', 'Linux').should('be.visible');

    // Verify the table is rendered
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('1').should('be.visible');
  });
});
