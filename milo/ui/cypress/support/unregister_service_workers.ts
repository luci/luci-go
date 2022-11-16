// Copyright 2021 The LUCI Authors.
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace Cypress {
    interface Chainable {
      /**
       * Unregister all service workers.
       */
      unregisterServiceWorkers: typeof unregisterServiceWorkers;
    }
  }
}

/**
 * Unregister all service workers.
 */
function unregisterServiceWorkers(url: string) {
  cy.visit(url, { failOnStatusCode: false });
  cy.window().then(async (win) => {
    const registrations = (await win.navigator?.serviceWorker?.getRegistrations()) || [];
    for (const registration of registrations) {
      await registration.unregister();
    }
  });
}

/**
 * Adds unregisterServiceWorkers commands to Cypress.
 */
export function addUnregisterServiceWorkersCommands() {
  Cypress.Commands.add('unregisterServiceWorkers', unregisterServiceWorkers);
}
