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

import { generateSidebarSections } from './sidebar_sections';

describe('generateSidebarSections', () => {
  it('generates Other Tools section with updated links', () => {
    const sections = generateSidebarSections();
    const otherTools = sections.find((s) => s.title === 'Other Tools');
    expect(otherTools).toBeDefined();

    expect(otherTools?.pages).toEqual([
      {
        label: 'Incidents (IRM)',
        url: 'http://go/fleetops-irm',
        icon: expect.anything(),
        external: true,
      },
      {
        label: 'Fleet Team',
        url: 'http://go/fleet',
        icon: expect.anything(),
        external: true,
      },
      {
        label: 'Nlyte Fleet Space Inventory',
        url: 'https://data.corp.google.com/sites/fo_quick_links/nlyte_fleet_space_inventory/',
        icon: expect.anything(),
        external: true,
      },
    ]);
  });
});
