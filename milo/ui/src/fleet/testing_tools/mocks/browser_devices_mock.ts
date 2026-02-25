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

import {
  BrowserDevice,
  ListBrowserDevicesResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { mockFetchRaw } from '@/testing_tools/jest_utils';

export const MOCK_BROWSER_DEVICE_1 = BrowserDevice.fromPartial({
  id: 'test-device-1',
  ufsLabels: { hostname: { values: ['test_device-hostname-1'] } },
  swarmingLabels: { os: { values: ['Linux'] } },
});

export const MOCK_BROWSER_DEVICE_2 = BrowserDevice.fromPartial({
  id: 'test-device-2',
  ufsLabels: { hostname: { values: ['test_device-hostname-2'] } },
  swarmingLabels: { os: { values: ['Windows'] } },
});

export function createMockListBrowserDevicesResponse(
  devices: readonly BrowserDevice[],
  nextPageToken: string,
) {
  return ListBrowserDevicesResponse.fromPartial({
    devices,
    nextPageToken,
  });
}

export function mockListBrowserDevices(
  devices: readonly BrowserDevice[],
  nextPageToken: string,
) {
  mockFetchRaw(
    (url) => url.includes('fleetconsole.FleetConsole/ListBrowserDevices'),
    ")]}'\n" +
      JSON.stringify(
        ListBrowserDevicesResponse.toJSON(
          createMockListBrowserDevicesResponse(devices, nextPageToken),
        ),
      ),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
        'Content-Type': 'application/json',
      },
    },
  );
}

export function mockErrorListingBrowserDevices() {
  mockFetchRaw(
    (url) => url.includes('fleetconsole.FleetConsole/ListBrowserDevices'),
    '',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
  );
}
