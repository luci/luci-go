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
};
