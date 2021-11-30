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

import { STUB_REQUEST_OPTIONS } from '../support/stub_prpc_services';

describe('Pixel Viewer', () => {
  it('should render pixel viewers correctly', () => {
    // modify prpc services to serve content cached from prod.
    cy.stubRequests(
      { url: 'https://cr-buildbucket-dev.appspot.com/prpc/**', method: 'POST' },
      'modified-buildbucket',
      STUB_REQUEST_OPTIONS
    );
    cy.stubRequests(
      { url: 'https://staging.results.api.cr.dev/prpc/**', method: 'POST' },
      'modified-resultdb',
      STUB_REQUEST_OPTIONS
    );
    cy.stubRequests({ url: 'https://localhost:8080/prpc/**', method: 'POST' }, 'modified-milo', STUB_REQUEST_OPTIONS);
    cy.stubRequests(
      { url: 'https://results.usercontent.cr.dev/**', method: 'GET' },
      'modified-artifacts',
      STUB_REQUEST_OPTIONS
    );

    cy.visit('/ui/p/chromium/builders/try/linux_layout_tests_composite_after_paint/51472/test-results', {
      qs: {
        q: 'highlight-es',
      },
    });

    cy.get('label').contains('Side by side').click();
    cy.scrollTo('topLeft');
    cy.get('#expected-image > img').invoke('prop', 'complete').should('be.true');

    // This error means that ResizeObserver was not able to deliver all
    // observations within a single animation frame. This error can be safely
    // ignored since we only care about the final element size. Browser throws
    // this error when cypress is in control of the frame.
    // Ignore the error as suggested by the cypress maintainer. See
    // https://github.com/quasarframework/quasar/issues/2233#issuecomment-492975745
    cy.on('uncaught:exception', (e) => !e.message.includes('ResizeObserver loop limit exceeded'));

    cy.get('#expected-image > img').click(8, 10);
    // Calling .click on an element causes the page to scroll. Reset the
    // scrolling position.
    cy.scrollTo('topLeft');
    cy.get('#pixel-viewer-grid');
    cy.matchImageSnapshot('pixel-viewer', { capture: 'viewport', clip: { x: 0, y: 0, width: 1000, height: 300 } });

    cy.get('#expected-image > img').then(([ele]) => {
      const rect = ele.getBoundingClientRect();
      cy.get('#expected-image > img').trigger('mousemove', { clientX: rect.x + 60, clientY: rect.y + 20 });
    });
    cy.matchImageSnapshot('pixel-viewer-after-move', {
      capture: 'viewport',
      clip: { x: 0, y: 0, width: 1000, height: 300 },
    });

    cy.get('#expected-image > img').click(8, 10);
    // Calling .click on an element causes the page to scroll. Reset the
    // scrolling position.
    cy.scrollTo('topLeft');
    cy.get('#pixel-viewer-grid').should('be.hidden');
  });
});
