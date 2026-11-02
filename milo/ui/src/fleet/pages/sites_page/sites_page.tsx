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

import { Box } from '@mui/material';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { enablePhysicalSites } from '@/fleet/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

// Physical Sites directory page (http://goto.google.com/fcon-labs-directory-design).
//
// Key design decisions:
// 1. Naming ("Physical Sites" at /ui/fleet/sites vs. "Physical Labs"):
//    Selected in a 6-candidate naming tournament to match FLOPS facility
//    terminology without colliding with /ui/fleet/labs experimental routes.
// 2. Primary identifier ("string id" building code vs. surrogate int64):
//    Sites use canonical building codes (e.g. "EM25", "MTV") as their primary
//    string ID (matching Device.id hostnames) paired with a human-readable
//    display name (e.g. "Palo Alto EM25").
export const SitesPage = () => {
  return <Box component="main" />;
};

export function Component() {
  const showPhysicalSites = useFeatureFlag(enablePhysicalSites);
  if (!showPhysicalSites) {
    return <PageNotFoundPage />;
  }

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-sites">
      <FleetHelmet pageTitle="Sites" />
      <RecoverableErrorBoundary key="fleet-sites-page">
        <LoggedInBoundary>
          <SitesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
