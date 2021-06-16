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

describe('Tooltip', () => {
  it('should render tooltip at the correct position', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252/overview');
    cy.get('.badge').first().trigger('mouseover');

    cy.scrollTo(0, '100px');
    cy.wait(100);
    cy.matchImageSnapshot('tooltip', { capture: 'viewport', clip: { x: 50, y: 500, width: 300, height: 100 } });

    cy.scrollTo(0, '200px');
    cy.wait(100);
    cy.matchImageSnapshot('tooltip-scrolled', {
      capture: 'viewport',
      clip: { x: 50, y: 400, width: 300, height: 100 },
    });
  });
});
