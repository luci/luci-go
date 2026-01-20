// Copyright 2024 The LUCI Authors.
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

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import {
  BrowserDevice,
  ListBrowserDevicesRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export type UseBrowserDeviceDataResult = {
  error?: unknown;
  isError: boolean;
  isLoading: boolean;
  device?: BrowserDevice;
};

/**
 * Queries for a device using ListBrowserDevices query with a single id filter
 * and serves cached data from previous ListDevices queries while loading.
 *
 * @param deviceId - the id of the device to query
 */
export const useBrowserDeviceData = (
  deviceId: string,
): UseBrowserDeviceDataResult => {
  const request = ListBrowserDevicesRequest.fromPartial({
    pageSize: 1,
    filter: stringifyFilters({ id: [deviceId] }),
  });

  const { data, error, isError, isLoading } = useBrowserDevices(request);

  return {
    error,
    isError,
    isLoading,
    device: data?.devices?.[0] ?? undefined,
  };
};
