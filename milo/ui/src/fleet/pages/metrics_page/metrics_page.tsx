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

export function MetricsPage() {
  return (
    <iframe
      src="https://dashboards.corp.google.com/_21972093_cc93_4547_9dd7_2954c9bf5aca"
      height="100%"
      width="100%"
      frameBorder="0"
      title="North Star Metrics"
    />
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-north-star-metrics">
      <FleetHelmet pageTitle="North Star Metrics" />
      <MetricsPage />
    </TrackLeafRoutePageView>
  );
}
