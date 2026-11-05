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

import { mockCypressAuth, mockPrpcEndpoint } from './common/utils';

describe('ChromeOS Devices Page', () => {
  beforeEach(() => {
    cy.clearLocalStorage();
    cy.window().then((win) => {
      win.indexedDB.deleteDatabase('keyval-store');
    });

    mockCypressAuth();

    mockPrpcEndpoint('GetDeviceDimensions', undefined, 'getDimensions');
    mockPrpcEndpoint(
      'ListDevices',
      {
        devices: [{ id: '1', labels: { os: { values: ['ChromeOS'] } } }],
        totalSize: 1,
      },
      'listDevices',
    );
    mockPrpcEndpoint('CountDevices', undefined, 'countDevices');
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

  it('should schedule autorepair with selected repairRequests', () => {
    mockPrpcEndpoint(
      'CheckPermission',
      { hasPermission: true },
      'checkPermission',
    );
    mockPrpcEndpoint(
      'ScheduleAutorepair',
      {
        sessionId: 'e2e-session-1',
        results: [
          {
            unitName: '1',
            taskUrl: 'https://ci.chromium.org/b/870001',
          },
        ],
      },
      'scheduleAutorepair',
    );

    cy.visit('/ui/fleet/p/chromeos/devices');
    cy.wait(['@listDevices', '@countDevices']);

    // Select the first device row
    cy.get('[data-testid^="select-checkbox-"]').first().click();
    cy.get('[data-testid^="select-checkbox-"]').first().should('be.checked');

    // Open the Run autorepair dialog
    cy.contains('button', 'Run autorepair').click();
    cy.wait('@checkPermission');

    // Select targeted repair actions and verify shivas command preview
    cy.get('input[aria-label="Re-image from USB (REIMAGE_BY_USBKEY)"]').check();
    cy.get(
      'input[aria-label="Audit internal storage (AUDIT_STORAGE)"]',
    ).check();
    cy.contains(
      'shivas update dut-state -dut-name 1 -repair-request REIMAGE_BY_USBKEY,AUDIT_STORAGE',
    ).should('be.visible');
    cy.contains('shivas repair 1').should('be.visible');

    // Confirm and verify the ScheduleAutorepair pRPC payload includes repairRequests
    cy.contains('button', 'Confirm').click({ force: true });
    cy.wait('@scheduleAutorepair').then((interception) => {
      const body =
        typeof interception.request.body === 'string'
          ? JSON.parse(interception.request.body)
          : interception.request.body;
      expect(body).to.deep.include({
        unitNames: ['1'],
        repairRequests: ['REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
      });
    });

    cy.contains('Autorepair results').should('be.visible');
  });
});
