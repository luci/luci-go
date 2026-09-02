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

import { getFeatureFlagValue } from '@/common/feature_flags';
import { enableChromeOsRepairsDashboard } from '@/fleet/features';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { generateSidebarSections } from './sidebar_sections';

jest.mock('@/common/feature_flags', () => ({
  ...jest.requireActual('@/common/feature_flags'),
  getFeatureFlagValue: jest.fn(),
}));

const mockGetFeatureFlagValue = getFeatureFlagValue as jest.Mock;

describe('generateSidebarSections', () => {
  beforeEach(() => {
    mockGetFeatureFlagValue.mockReset();
    mockGetFeatureFlagValue.mockImplementation((flag) => {
      if (flag === enableChromeOsRepairsDashboard) return true;
      return false;
    });
  });

  it('enables Admin tasks for ChromeOS platform', () => {
    const sections = generateSidebarSections(Platform.CHROMEOS);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    expect(labHealth).toBeDefined();

    const adminTasksPage = labHealth?.pages.find(
      (p) => p.label === 'Admin tasks',
    );
    expect(adminTasksPage).toBeDefined();
    expect(adminTasksPage?.disabled).toBeFalsy();
    expect(adminTasksPage?.url).toBe('/ui/fleet/p/chromeos/admin-tasks');
    expect(adminTasksPage?.tooltip).toBeUndefined();
  });

  it('hides Admin tasks for Android platform', () => {
    const sections = generateSidebarSections(Platform.ANDROID);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    const adminTasksPage = labHealth?.pages.find(
      (p) => p.label === 'Admin tasks',
    );
    expect(adminTasksPage).toBeUndefined();
  });

  it('disables Repairs for Chromium platform with tooltip', () => {
    const sections = generateSidebarSections(Platform.CHROMIUM);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    expect(labHealth).toBeDefined();

    const repairsPage = labHealth?.pages.find((p) => p.label === 'Repairs');
    expect(repairsPage).toBeDefined();
    expect(repairsPage?.disabled).toBeTruthy();
    expect(repairsPage?.tooltip).toBe(
      'Repairs for Chrome Browser are not available yet',
    );
  });

  it('hides Admin tasks for Chromium platform', () => {
    const sections = generateSidebarSections(Platform.CHROMIUM);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    expect(
      labHealth?.pages.find((p) => p.label === 'Admin tasks'),
    ).toBeUndefined();
  });

  it('hides Admin tasks for Pixel platform', () => {
    const sections = generateSidebarSections(Platform.PIXEL);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    expect(
      labHealth?.pages.find((p) => p.label === 'Admin tasks'),
    ).toBeUndefined();
  });

  it('enables Admin tasks when platform is undefined (default)', () => {
    const sections = generateSidebarSections();
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    const adminTasksPage = labHealth?.pages.find(
      (p) => p.label === 'Admin tasks',
    );
    expect(adminTasksPage).toBeDefined();
    expect(adminTasksPage?.disabled).toBeFalsy();
    expect(adminTasksPage?.url).toBe('/ui/fleet/p/chromeos/admin-tasks');
  });

  it('passes pendingAdminTasksCount badgeCount to Admin tasks page', () => {
    const sections = generateSidebarSections(Platform.CHROMEOS, 5);
    const labHealth = sections.find((s) => s.title === 'Lab Health');
    const adminTasksPage = labHealth?.pages.find(
      (p) => p.label === 'Admin tasks',
    );
    expect(adminTasksPage?.badgeCount).toBe(5);
  });

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

  describe('Repairs link', () => {
    it('is enabled for Android by default', () => {
      const sections = generateSidebarSections(Platform.ANDROID);
      const labHealth = sections.find((s) => s.title === 'Lab Health');
      const repairsPage = labHealth?.pages.find((p) => p.label === 'Repairs');
      expect(repairsPage).toBeDefined();
      expect(repairsPage?.disabled).toBeFalsy();
    });

    it('is disabled for ChromeOS when feature flag is off', () => {
      mockGetFeatureFlagValue.mockReturnValue(false);
      const sections = generateSidebarSections(Platform.CHROMEOS);
      const labHealth = sections.find((s) => s.title === 'Lab Health');
      const repairsPage = labHealth?.pages.find((p) => p.label === 'Repairs');
      expect(repairsPage).toBeDefined();
      expect(repairsPage?.disabled).toBeTruthy();
    });

    it('is enabled for ChromeOS when feature flag is on', () => {
      mockGetFeatureFlagValue.mockImplementation((flag) => {
        if (flag === enableChromeOsRepairsDashboard) return true;
        return false;
      });
      const sections = generateSidebarSections(Platform.CHROMEOS);
      const labHealth = sections.find((s) => s.title === 'Lab Health');
      const repairsPage = labHealth?.pages.find((p) => p.label === 'Repairs');
      expect(repairsPage).toBeDefined();
      expect(repairsPage?.disabled).toBeFalsy();
      expect(repairsPage?.url).toBe('/ui/fleet/p/chromeos/repairs');
    });

    it('is enabled for Pixel by default', () => {
      const sections = generateSidebarSections(Platform.PIXEL);
      const labHealth = sections.find((s) => s.title === 'Lab Health');
      const repairsPage = labHealth?.pages.find((p) => p.label === 'Repairs');
      expect(repairsPage).toBeDefined();
      expect(repairsPage?.disabled).toBeFalsy();
      expect(repairsPage?.url).toBe('/ui/fleet/p/pixel/repairs');
    });
  });

  it('includes Product Catalog by default for all callers', () => {
    const sections = generateSidebarSections();
    const requests = sections.find((s) => s.title === 'Resource Requests');
    const catalogPage = requests?.pages.find(
      (p) => p.label === 'Product Catalog',
    );
    expect(catalogPage).toBeDefined();
    expect(catalogPage?.url).toBe('/ui/fleet/labs/catalog');
  });
});
