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
import { genericGiven } from '@/fleet/testing_tools/integration/fleet_integration_test';
import {
  GetDefaultQuotaResponse,
  GetDeviceDimensionsResponse,
  GetFleetAvailabilityTrendsResponse,
  ListHealthSlicesResponse,
  ListModelQuotaOverridesResponse,
  ListSupportRiskIncidentsResponse,
  ModelQuotaOverride,
  TrendlineMetricType,
  TrendlineSeries,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const given = {
  ...genericGiven,

  unauthenticated() {
    genericGiven.authenticatedAs(ANONYMOUS_IDENTITY, '');
  },

  deviceDimensions({
    models = ['volteer', 'brya'],
    pools = ['DUT_POOL_QUOTA'],
  }: {
    models?: string[];
    pools?: string[];
  } = {}) {
    genericGiven.rpcResponse(
      'GetDeviceDimensions',
      GetDeviceDimensionsResponse.toJSON(
        GetDeviceDimensionsResponse.fromPartial({
          baseDimensions: {},
          labels: {
            'label-model': { values: models },
            'label-pool': { values: pools },
          },
        }),
      ),
    );
  },

  availabilityTrends({
    metricType = TrendlineMetricType.AVAILABILITY,
    series = [],
  }: {
    metricType?: TrendlineMetricType;
    series?: Partial<TrendlineSeries>[];
  } = {}) {
    genericGiven.rpcResponse(
      'GetFleetAvailabilityTrends',
      GetFleetAvailabilityTrendsResponse.toJSON(
        GetFleetAvailabilityTrendsResponse.fromPartial({
          metricType,
          series,
        }),
      ),
    );
  },

  quotaOverrides(overrides: Partial<ModelQuotaOverride>[] = []) {
    genericGiven.rpcResponse(
      'ListModelQuotaOverrides',
      ListModelQuotaOverridesResponse.toJSON(
        ListModelQuotaOverridesResponse.fromPartial({
          overrides,
        }),
      ),
    );
  },
};

beforeEach(() => {
  given.deviceDimensions();
  given.availabilityTrends();
  given.quotaOverrides();
  genericGiven.rpcResponse(
    'ListHealthSlices',
    ListHealthSlicesResponse.toJSON(
      ListHealthSlicesResponse.fromPartial({
        healthSlices: [{ id: 'all-fleet', name: 'All Fleet', filter: '' }],
      }),
    ),
  );
  genericGiven.rpcResponse(
    'GetDefaultQuota',
    GetDefaultQuotaResponse.toJSON(
      GetDefaultQuotaResponse.fromPartial({
        defaultQuota: 49,
      }),
    ),
  );
  genericGiven.rpcResponse(
    'ListSupportRiskIncidents',
    ListSupportRiskIncidentsResponse.toJSON(
      ListSupportRiskIncidentsResponse.fromPartial({
        incidents: [
          {
            id: '1',
            model: 'volteer',
            title: 'Battery controller defect',
            buganizerId: 'b/312456789',
          },
        ],
      }),
    ),
  );
});
