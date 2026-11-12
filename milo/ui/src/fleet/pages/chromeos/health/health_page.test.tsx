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

import { TrendlineMetricType } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { given } from './health_page_given';
import { then } from './health_page_then';
import { when } from './health_page_when';

describe('<HealthPage /> (Full Page Integration with Fake pRPC Backend)', () => {
  it('renders overview page with header, Configuration button, FilterBar, and Support Risk Incidents panel', async () => {
    when.pageIsRendered();

    await then.overviewDashboardIsVisible();
    await then.configurationButtonIsVisible();
  });

  it('switches slice baseline chip to Enrolled Baseline when non-model filter is applied', async () => {
    given.initialUrl('/?filters=pool%20%3D%20%22DUT_POOL_QUOTA%22');
    given.availabilityTrends({
      metricType: TrendlineMetricType.HEALTH,
      series: [],
    });

    when.pageIsRendered();

    await then.sliceBaselineChipIs('all-fleet', 'Enrolled Baseline');
  });

  it('applies model filter when Show button on Support Risk incident is clicked', async () => {
    when.pageIsRendered();

    await when.userClicksButton(/^show$/i);

    await then.filterChipIsVisible('1 | Model IN volteer');
    then.searchParamIs('filters', '(model = "volteer")');
  });

  it('navigates to Configuration view showing the quota card and returns on Back', async () => {
    when.pageIsRendered();

    await when.userClicksButton(/^configuration$/i);

    await then.textIsVisible('Fleet Health Configuration');
    await then.textIsVisible('Global Default Expected Quota');
    await then.textIsVisible('Manual Model Quota Overrides');

    await when.userClicksButton(/back to fleet overview/i);

    await then.textIsVisible('ChromeOS Fleet Health Metrics');
    then.textIsNotVisible('Global Default Expected Quota');
  });

  it('hides Configuration button when user lacks permission', async () => {
    given.permission(false);

    when.pageIsRendered();

    await then.textIsVisible('ChromeOS Fleet Health Metrics');
    await then.configurationButtonIsNotVisible();
  });

  it('does not display configuration view when tab=configuration is accessed without permission', async () => {
    given.permission(false);
    given.initialUrl('/?tab=configuration');

    when.pageIsRendered();

    await then.textIsVisible('ChromeOS Fleet Health Metrics');
    then.textIsNotVisible('Fleet Health Configuration');
  });

  it('renders wrapped Component with analytics and error boundaries', async () => {
    when.routeComponentIsRendered();

    await then.textIsVisible('ChromeOS Fleet Health Metrics');
    then.trackedAnalyticsEvent('page_view', {
      appId: undefined,
      content_group: 'fleet-console-chromeos-health',
      page_location: 'http://localhost/',
      page_title: 'ChromeOS Fleet Health Metrics | Fleet Console',
    });
  });

  it('requires login in route Component when user is anonymous', async () => {
    given.mountedPath('/ui/fleet/p/chromeos/health');
    given.initialUrl('/ui/fleet/p/chromeos/health');
    given.unauthenticated();

    when.routeComponentIsRendered();

    await then.textIsVisible('Sign in to Fleet Console');
    then.textIsNotVisible('ChromeOS Fleet Health Metrics');
  });

  it('preserves exact model casing in ranking rows, device links, and row click filter selection', async () => {
    given.deviceDimensions({
      models: ['Demo-Model-One', 'MOCK-MODEL-TWO', 'mock-model-two'],
      pools: ['DUT_POOL_QUOTA'],
    });
    given.availabilityTrends({
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'Demo-Model-One',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.62 }],
        },
        {
          name: 'MOCK-MODEL-TWO',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.71 }],
        },
        {
          name: 'mock-model-two',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.84 }],
        },
      ],
    });

    when.pageIsRendered();

    await then.rankingRowHasDeviceListFilter(
      'Demo-Model-One',
      '(labels."label-model" = "Demo-Model-One")',
    );
    await then.rankingRowHasDeviceListFilter(
      'MOCK-MODEL-TWO',
      '(labels."label-model" = "MOCK-MODEL-TWO")',
    );
    await then.rankingRowHasDeviceListFilter(
      'mock-model-two',
      '(labels."label-model" = "mock-model-two")',
    );

    await when.userClicksRankingRow('MOCK-MODEL-TWO');
    await then.filterChipIsVisible('1 | Model IN MOCK-MODEL-TWO');
    then.searchParamIs('filters', '(model = "MOCK-MODEL-TWO")');
    then.rpcWasCalledWith('GetFleetAvailabilityTrends', {
      filter: '(model = "MOCK-MODEL-TWO")',
    });

    await when.userClicksRankingRow('mock-model-two');
    await then.filterChipIsVisible('1 | Model IN mock-model-two');
    then.searchParamIs('filters', '(model = "mock-model-two")');
    then.rpcWasCalledWith('GetFleetAvailabilityTrends', {
      filter: '(model = "mock-model-two")',
    });
  });

  it('treats model quota overrides with distinct casing as separate models in Configuration view', async () => {
    given.initialUrl('/?tab=configuration');
    given.quotaOverrides([
      { id: '1', model: 'demo-model-one', overriddenExpectedQuantity: 15 },
    ]);

    when.pageIsRendered();
    await then.textIsVisible('demo-model-one');

    await when.userEntersQuotaOverride('  demo-model-one  ', '15');
    then.setOverrideButtonIsDisabled();

    await when.userEntersQuotaOverride('DEMO-MODEL-ONE', '15');
    then.setOverrideButtonIsEnabled();

    await when.userClicksButton(/set override/i);
    then.rpcWasCalledWith('SetModelQuotaOverride', {
      model: 'DEMO-MODEL-ONE',
      overriddenExpectedQuantity: 15,
    });
  });
});
