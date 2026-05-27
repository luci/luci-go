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

describe('Android Devices Page', () => {
  beforeEach(() => {
    cy.intercept('**', (req) => {
      req.continue((res) => {
        if (res.statusCode >= 400) {
          /* eslint-disable no-console */
          console.error(`\n--- NETWORK ERROR ---`);
          console.error(`URL: ${req.url}`);
          console.error(`Status: ${res.statusCode}`);
          console.error(`Body: ${JSON.stringify(res.body)}`);
          console.error(`----------------------\n`);
          /* eslint-enable no-console */
        }
      });
    });

    const dimensionsData = {
      baseDimensions: {
        run_target: { values: ['a04e'] },
        state: { values: ['LAMEDUCK'] },
      },
      labels: {},
    };
    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/GetDeviceDimensions',
      dimensionsData,
      'getDimensions',
    );

    mockPrpc(
      '**/prpc/fleetconsole.FleetConsole/ListAndroidDevices',
      {
        devices: [{ id: '1', runTarget: 'a04e', state: 'LAMEDUCK' }],
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

  it('should not infinite loop or crash when loading with multiple filters', () => {
    // The bug was triggered when reloading the page with more than 2 filters in the URL.
    // We use the example from the commit message to reproduce the exact scenario.
    const filters = encodeURIComponent(
      '"run_target" = ("a04e") AND "state" = ("LAMEDUCK")',
    );
    const targetUrl = `/ui/fleet/p/android/devices?filters=${filters}`;

    // Navigate to the page with the filters pre-applied in the URL
    cy.visit(targetUrl);

    // Wait for network requests to settle
    cy.wait(['@getDimensions', '@listDevices', '@countDevices']);

    // Verify the URL remains stable and contains our filters (not infinitely rewritten)
    cy.url().should('include', 'filters');

    // 4. Verify the page didn't crash and the filter chips rendered their full values.
    cy.get('.MuiChip-root').contains('run_target').should('be.visible');
    cy.get('.MuiChip-root').contains('a04e').should('be.visible');
    cy.get('.MuiChip-root').contains('State').should('be.visible');
    cy.get('.MuiChip-root').contains('LAMEDUCK').should('be.visible');

    // 5. Ensure the main UI table is still present and contains data.
    cy.get('table').should('be.visible');
    cy.get('table').find('td').contains('1').should('be.visible');
  });
});
