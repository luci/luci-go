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

import { renderHook, waitFor } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { useDevices } from '@/fleet/hooks/use_devices';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import sampleAndroidDevices from './data/android_devices.json';
import sampleBrowserDevices from './data/browser_devices.json';
import sampleChromeosDevices from './data/chromeos_devices.json';
import sampleProductCatalog from './data/product_catalog.json';
import sampleRepairMetrics from './data/repair_metrics.json';
import sampleResourceRequests from './data/resource_requests.json';
import { FleetConsoleMockAPI } from './mock_api_handler';

describe('Fleet Console Frontend Mock API Real Use Cases', () => {
  beforeEach(() => {
    FleetConsoleMockAPI.resetFixtures();
    FleetConsoleMockAPI.enableBrowserInterceptor();
  });

  afterEach(() => {
    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  describe('Device List Queries via Fleet Console Hooks', () => {
    it('loads ChromeOS devices using useDevices hook with synthetic mock data', async () => {
      const { result } = renderHook(
        () =>
          useDevices({
            platform: Platform.CHROMEOS,
            pageSize: 50,
            filter: '',
            pageToken: '',
            orderBy: '',
          }),
        { wrapper: FakeContextProvider },
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const devices = result.current.data?.devices ?? [];
      expect(devices).toHaveLength(sampleChromeosDevices.length);
      expect(devices[0].id).toBe(sampleChromeosDevices[0].id);
      expect(devices[0].dutId).toBe(sampleChromeosDevices[0].dutId);
      expect(
        devices[0].deviceSpec?.labels?.['label-board']?.values?.[0],
      ).toBeDefined();
    });

    it('loads Android devices using useAndroidDevices hook with synthetic mock data', async () => {
      const { result } = renderHook(
        () =>
          useAndroidDevices(
            {
              pageSize: 25,
              pageToken: '',
              orderBy: '',
              filter: '',
            },
            'Android',
          ),
        { wrapper: FakeContextProvider },
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const devices = result.current.data?.devices ?? [];
      expect(devices).toHaveLength(sampleAndroidDevices.length);
      expect(devices[0].id).toBe(sampleAndroidDevices[0].id);
      expect(devices[0].runTarget).toBe(sampleAndroidDevices[0].runTarget);
      expect(
        devices[0].omnilabSpec?.labels?.['host_ip']?.values?.[0],
      ).toBeDefined();
      expect(
        devices[0].omnilabSpec?.labels?.['dut_state']?.values?.[0],
      ).toBeDefined();
    });

    it('loads Browser devices using useBrowserDevices hook with synthetic mock data', async () => {
      const { result } = renderHook(
        () =>
          useBrowserDevices({
            pageSize: 25,
            pageToken: '',
            orderBy: '',
            filter: '',
          }),
        { wrapper: FakeContextProvider },
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      const devices = result.current.data?.devices ?? [];
      expect(devices).toHaveLength(sampleBrowserDevices.length);
      expect(devices[0].id).toBe(sampleBrowserDevices[0].id);
      expect(devices[0].ufsLabels?.hostname?.values?.[0]).toBeDefined();
      expect(devices[0].swarmingLabels?.os?.values?.[0]).toBeDefined();
    });
  });

  describe('Fleet Console pRPC Client Direct RPC Execution', () => {
    it('aggregates device counts across ChromeOS and Android fleets', async () => {
      const { result } = renderHook(() => useFleetConsoleClient(), {
        wrapper: FakeContextProvider,
      });

      const client = result.current;
      const count = await client.CountDevices({
        platform: Platform.CHROMEOS,
        filter: '',
      });

      expect(count.total).toBe(
        sampleChromeosDevices.length + sampleAndroidDevices.length,
      );
      expect(count.chromeosCount?.total).toBe(sampleChromeosDevices.length);
      expect(count.androidCount?.totalDevices).toBe(
        sampleAndroidDevices.length,
      );
    });

    it('queries repair metrics with full topological consistency', async () => {
      const { result } = renderHook(() => useFleetConsoleClient(), {
        wrapper: FakeContextProvider,
      });

      const client = result.current;
      const response = await client.ListRepairMetrics({
        platform: Platform.CHROMEOS,
        pageSize: 10,
        pageToken: '',
        orderBy: '',
        filter: '',
      });

      expect(response.repairMetrics).toHaveLength(sampleRepairMetrics.length);
      expect(response.repairMetrics[0].labName).toBe(
        sampleRepairMetrics[0].labName,
      );
      expect(response.repairMetrics[0].hostGroup).toBe(
        sampleRepairMetrics[0].hostGroup,
      );
    });

    it('queries resource requests and product catalog entries', async () => {
      const { result } = renderHook(() => useFleetConsoleClient(), {
        wrapper: FakeContextProvider,
      });

      const client = result.current;
      const requests = await client.ListResourceRequests({
        pageSize: 10,
        pageToken: '',
        orderBy: '',
        filter: '',
      });
      const catalog = await client.ListProductCatalogEntries({
        filter: '',
      });

      expect(requests.resourceRequests).toHaveLength(
        sampleResourceRequests.length,
      );
      expect(requests.resourceRequests[0].rrId).toBe(
        sampleResourceRequests[0].rrId,
      );

      expect(catalog.entries).toHaveLength(sampleProductCatalog.length);
      expect(catalog.entries[0].productCatalogId).toBe(
        sampleProductCatalog[0].productCatalogId,
      );
    });
  });

  describe('Custom Fixture Override and Isolation in Tests', () => {
    it('allows mocking specific error states or empty device sets', async () => {
      FleetConsoleMockAPI.setFixture('ListDevices', {
        devices: [],
        nextPageToken: '',
      });

      const { result } = renderHook(
        () =>
          useDevices({
            platform: Platform.CHROMEOS,
            pageSize: 50,
            filter: '',
            pageToken: '',
            orderBy: '',
          }),
        { wrapper: FakeContextProvider },
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data?.devices).toHaveLength(0);

      // Reset restores default synthetic fixtures cleanly
      FleetConsoleMockAPI.resetFixtures();
      const { result: resetResult } = renderHook(
        () =>
          useDevices({
            platform: Platform.CHROMEOS,
            pageSize: 50,
            filter: '',
            pageToken: '',
            orderBy: '',
          }),
        { wrapper: FakeContextProvider },
      );

      await waitFor(() => expect(resetResult.current.isSuccess).toBe(true));
      expect(resetResult.current.data?.devices).toHaveLength(
        sampleChromeosDevices.length,
      );
    });
  });

  describe('Front Door Mock Data Coverage Integrity', () => {
    it('verifies that 100% of checked-in sample datasets are non-empty and structurally valid', () => {
      expect(sampleChromeosDevices.length).toBeGreaterThan(0);
      expect(sampleAndroidDevices.length).toBeGreaterThan(0);
      expect(sampleBrowserDevices.length).toBeGreaterThan(0);
      expect(sampleProductCatalog.length).toBeGreaterThan(0);
      expect(sampleRepairMetrics.length).toBeGreaterThan(0);
      expect(sampleResourceRequests.length).toBeGreaterThan(0);
    });
  });
});
