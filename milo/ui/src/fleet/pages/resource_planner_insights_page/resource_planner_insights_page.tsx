// Copyright 2025 The LUCI Authors.
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

import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

export function ResourcePlannerInsightsPage() {
  return (
    <iframe
      src="https://dashboards.corp.google.com/_3e8159b6_1995_4800_a0a1_5f184f1f11ff"
      height="100%"
      width="100%"
      frameBorder="0"
      title="Resource Planner Insights"
    />
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-resource-planner-insights">
      <FleetHelmet pageTitle="Resource Planner Insights" />
      <ResourcePlannerInsightsPage />
    </TrackLeafRoutePageView>
  );
}
