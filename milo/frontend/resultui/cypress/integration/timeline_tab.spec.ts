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

describe('Timeline Tab', () => {
  it('should render timeline correctly', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15252/timeline');
    cy.get('#timeline');
    cy.matchImageSnapshot('timeline-tab');
  });

  it('can handle builds with no steps', () => {
    cy.visit('/p/chromium/builders/ci/linux-rel-swarming/15467/timeline');
    cy.get('#no-steps').contains('No steps were run.');
  });
});
