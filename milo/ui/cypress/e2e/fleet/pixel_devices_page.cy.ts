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

describe('Pixel Devices Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('featureFlag:fleet-console:pte-support', 'on');
    });

    // Mock auth state to be a Googler so that protected routes are accessible.
    cy.intercept('GET', '**/auth/openid/state', {
      body: {
        identity: 'user:user@google.com',
        email: 'user@google.com',
      },
    });

    const dimensionsData = {
      baseDimensions: {
        run_target: { values: ['coral'] },
        state: { values: ['LAMEDUCK'] },
      },
      labels: {
        model: { values: ['Pixel 8'] },
        host_group: { values: ['pte_labs'] },
      },
    };
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetDeviceDimensions',
      dimensionsData,
      'getDimensions',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListAndroidDevices',
      {
        devices: [
          {
            id: 'pixel-1',
            runTarget: 'coral',
            state: 'LAMEDUCK',
            omnilabSpec: {
              labels: {
                model: { values: ['Pixel 8'] },
                host_group: { values: ['pte_labs'] },
              },
            },
          },
        ],
        totalSize: 1,
      },
      'listDevices',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/CountDevices',
      {
        androidCount: {
          totalDevices: 1,
          totalHosts: 1,
          idleDevices: 0,
          busyDevices: 0,
          missingDevices: 0,
          failedDevices: 0,
          dirtyDevices: 0,
          preppingDevices: 0,
          dyingDevices: 0,
          initDevices: 0,
          lameduckDevices: 1,
          labRunningHosts: 1,
          labMissingHosts: 0,
        },
      },
      'countDevices',
    );
  });

  it('should load and render table', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);

    // Wait for network requests to settle
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Verify table is visible and contains data
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('pixel-1').should('be.visible');
    cy.get('table').find('td').contains('coral').should('be.visible');
  });

  it('should redirect from root pixel platform path to devices', () => {
    cy.visit('/ui/fleet/p/pixel');

    // Wait for network requests to settle
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Verify redirected URL contains /devices
    cy.url().should('include', '/ui/fleet/p/pixel/devices');

    // Verify table is rendered
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('pixel-1').should('be.visible');
  });

  it('should support row selection', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Locate the first row checkbox and click it
    cy.get('[data-testid^="select-checkbox-"]').first().click();

    // Verify the row becomes selected/checked
    cy.get('[data-testid^="select-checkbox-"]').first().should('be.checked');
  });

  it('should load with filters and render chips', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl, {
      qs: {
        filters: '"run_target" = ("coral") AND "state" = ("LAMEDUCK")',
      },
    });

    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Verify URL contains filters
    cy.url().should('include', 'filters');

    // Verify filter chips are rendered
    cy.get('.MuiChip-root').contains('run_target').should('be.visible');
    cy.get('.MuiChip-root').contains('coral').should('be.visible');
    cy.get('.MuiChip-root').contains('State').should('be.visible');
    cy.get('.MuiChip-root').contains('LAMEDUCK').should('be.visible');

    // Verify table is rendered with device data
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('pixel-1').should('be.visible');
  });

  it('should filter devices using filter bar dropdown', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Open filter dropdown and select run_target -> coral
    cy.get('input[placeholder*="Add a filter"]').click();
    cy.get('[role="menuitem"]').should('have.length.gt', 0);
    cy.get('[role="menuitem"]').contains('run_target').click();
    cy.get('[role="menuitem"]').contains('coral').click();
    cy.contains('button', 'Apply').click();

    // Verify chip rendered
    cy.contains('[role="button"]', 'run_target').should('be.visible');
    cy.contains('[role="button"]', 'coral').should('be.visible');
  });

  it('should navigate to device details when clicking device ID link in table', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Click device ID link in table
    cy.get('table').find('td').contains('pixel-1').click();

    // Verify URL navigation to details
    cy.url().should('include', '/ui/fleet/p/pixel/devices/pixel-1');
    cy.contains('Device details:').should('be.visible');
    cy.contains('Pixel 8').should('be.visible');
  });

  it('should load device details page directly', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices/pixel-1';

    cy.visit(targetUrl);
    cy.wait('@listDevices');

    // Verify Device details header and content
    cy.contains('Device details:').should('be.visible');
    cy.get('table').should('be.visible');
    cy.contains('model').should('be.visible');
    cy.contains('Pixel 8').should('be.visible');
  });

  it('should open and close keyboard shortcuts modal with ? shortcut', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Press ? to open keyboard shortcuts modal
    cy.get('body').type('?');
    cy.get('[role="dialog"]').should('be.visible');
    cy.get('[role="dialog"]')
      .contains('Keyboard Shortcuts')
      .should('be.visible');

    // Close modal via close button
    cy.get('[role="dialog"]').find('button[aria-label="close"]').click();
    cy.get('[role="dialog"]').should('not.exist');
  });

  it('should focus search bar with / shortcut and ignore shortcuts while typing', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Press / to focus search bar
    cy.get('body').type('/');
    cy.get('input[placeholder*="Add a filter"]').should('be.focused');

    // Type characters into search bar - shortcuts like ?, c, / should not trigger while typing
    cy.focused().type('?c');
    cy.get('[role="dialog"]').should('not.exist');
    cy.contains('Reset defaults').should('not.exist');
    cy.get('input[placeholder*="Add a filter"]').should('have.value', '?c');
  });

  it('should open column picker with c shortcut', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Press c to open column picker
    cy.get('body').type('c');
    cy.contains('Reset defaults').should('be.visible');

    // Press escape to close
    cy.get('body').type('{esc}');
    cy.contains('Reset defaults').should('not.exist');
  });

  it('should switch platforms using keyboard shortcuts', () => {
    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Press g then a to switch to Android platform
    cy.get('body').type('ga');
    cy.url().should('include', '/ui/fleet/p/android/devices');

    // Press g then x to switch back to Pixel platform
    cy.get('body').type('gx');
    cy.url().should('include', '/ui/fleet/p/pixel/devices');
  });

  it('should navigate to repairs page using keyboard shortcut', () => {
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetRepairMetricsDimensions',
      { dimensions: {} },
      'getRepairDimensions',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListRepairMetrics',
      {
        repairMetrics: [
          { labName: 'lab1', totalDevices: 10, devicesOffline: 2 },
        ],
      },
      'listRepairMetrics',
    );
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/CountRepairMetrics',
      { total: 1 },
      'countRepairMetrics',
    );

    const targetUrl = '/ui/fleet/p/pixel/devices';

    cy.visit(targetUrl);
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Press g then r to navigate to repairs page
    cy.get('body').type('gr');
    cy.url().should('include', '/ui/fleet/p/pixel/repairs');
    cy.contains('Repair metrics').should('be.visible');

    // Press g then d to navigate back to devices page
    cy.get('body').type('gd');
    cy.url().should('include', '/ui/fleet/p/pixel/devices');
  });
});
