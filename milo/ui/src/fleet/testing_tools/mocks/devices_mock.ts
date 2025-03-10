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

import fetchMock from 'fetch-mock-jest';

import {
  Device,
  ListDevicesResponse,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const LIST_DEVICES_ENDPOINT = `https://${SETTINGS.fleetConsole.host}/prpc/fleetconsole.FleetConsole/ListDevices`;

export const MOCK_DEVICE_1 = Device.fromPartial({
  id: 'test-device-1',
  dutId: '312323',
  deviceSpec: { labels: { label1: { values: ['a', 'b', 'c'] } } },
});

export const MOCK_DEVICE_2 = Device.fromPartial({
  id: 'test-device-1',
  dutId: 'v314233',
  deviceSpec: { labels: { label1: { values: ['d', 'e', 'f'] } } },
});

export function createMockListDevicesResponse(
  devices: readonly Device[],
  nextPageToken: string,
) {
  return ListDevicesResponse.fromPartial({
    devices,
    nextPageToken,
  });
}

export function mockListDevices(
  devices: readonly Device[],
  nextPageToken: string,
) {
  fetchMock.post(LIST_DEVICES_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body:
      ")]}'\n" +
      JSON.stringify(
        ListDevicesResponse.toJSON(
          createMockListDevicesResponse(devices, nextPageToken),
        ),
      ),
  });
}

export function mockErrorListingDevices() {
  fetchMock.post(LIST_DEVICES_ENDPOINT, {
    headers: {
      'X-Prpc-Grpc-Code': '2',
    },
  });
}
