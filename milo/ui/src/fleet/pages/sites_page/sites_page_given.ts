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

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { enablePhysicalSites } from '@/fleet/features';
import { genericGiven } from '@/fleet/testing_tools/integration/fleet_integration_test';
import { fakeFleetConsoleBackend } from '@/fleet/testing_tools/mock_api/fake_fleet_console_backend';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const given = {
  ...genericGiven,

  physicalSitesEnabled() {
    genericGiven.featureFlag(enablePhysicalSites, true);
  },

  physicalSitesDisabled() {
    genericGiven.featureFlag(enablePhysicalSites, false);
  },

  unauthenticated() {
    genericGiven.authenticatedAs(ANONYMOUS_IDENTITY, '');
  },

  withoutAdminTaskPermission() {
    fakeFleetConsoleBackend.permissions.setGroupPermission(
      'mdb/fleet-console-admin-tasks-policy',
      false,
    );
  },

  sites(items: readonly Partial<PhysicalSite>[]) {
    fakeFleetConsoleBackend.physicalSites.seed(items);
  },

  platformCounts(counts: {
    chromeos?: { total: number; ready: number };
    android?: { totalDevices: number; inService: number };
    browser?: { total: number };
  }) {
    if (counts.chromeos) {
      genericGiven.rpcResponse('CountDevices', {
        total: counts.chromeos.total,
        deviceState: {
          ready: counts.chromeos.ready,
          needRepair: 0,
          repairFailed: 0,
          needManualRepair: counts.chromeos.total - counts.chromeos.ready,
        },
      });
    }
    if (counts.android) {
      genericGiven.rpcResponse('CountAndroidDevices', {
        totalDevices: counts.android.totalDevices,
        healthCategoryInService: {
          total: counts.android.inService,
        },
      });
    }
    if (counts.browser) {
      genericGiven.rpcResponse('CountBrowserDevices', {
        total: counts.browser.total,
      });
    }
  },

  listSitesError(message = 'Failed to fetch sites') {
    genericGiven.rpcError('ListPhysicalSites', 13, message);
  },

  createSiteError(message = 'Failed to create site') {
    genericGiven.rpcError('CreatePhysicalSite', 13, message);
  },
};
