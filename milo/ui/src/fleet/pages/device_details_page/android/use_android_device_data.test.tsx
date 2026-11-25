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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import { ReactNode } from 'react';

import * as useAndroidDevicesModule from '@/fleet/hooks/use_android_devices';
import {
  AndroidDevice,
  ListAndroidDevicesRequest,
  ListAndroidDevicesResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useAndroidDeviceData } from './use_android_device_data';

const LIST_QUERY_KEY = ['list-android-devices'] as const;

describe('useAndroidDeviceData', () => {
  let queryClient: QueryClient;
  let useAndroidDevicesSpy: jest.SpyInstance;

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  beforeEach(() => {
    jest.clearAllMocks();
    queryClient = new QueryClient();
    jest
      .spyOn(useAndroidDevicesModule, 'useListAndroidDevicesQueryKey')
      .mockReturnValue(LIST_QUERY_KEY);
    useAndroidDevicesSpy = jest
      .spyOn(useAndroidDevicesModule, 'useAndroidDevices')
      .mockReturnValue({
        data: undefined,
        error: null,
        isError: false,
        isLoading: true,
      } as ReturnType<typeof useAndroidDevicesModule.useAndroidDevices>);
  });

  it('includes fc_machine_type in filter when machineType is provided', () => {
    renderHook(() => useAndroidDeviceData('shared-id', 'Android', 'host'), {
      wrapper,
    });

    const request = useAndroidDevicesSpy.mock
      .calls[0][0] as ListAndroidDevicesRequest;
    expect(request.filter).toContain('id = "shared-id"');
    expect(request.filter).toContain('fc_machine_type = "host"');
  });

  it('omits fc_machine_type from filter when machineType is not provided', () => {
    renderHook(() => useAndroidDeviceData('device-1', 'Android'), { wrapper });

    const request = useAndroidDevicesSpy.mock
      .calls[0][0] as ListAndroidDevicesRequest;
    expect(request.filter).toContain('id = "device-1"');
    expect(request.filter).not.toContain('fc_machine_type');
  });

  it('disambiguates cached devices with shared id by fc_machine_type while loading', () => {
    const deviceRow = AndroidDevice.fromPartial({
      id: 'shared-id',
      omnilabSpec: {
        labels: {
          fc_machine_type: { values: ['device'] },
          state: { values: ['IDLE'] },
        },
      },
    });
    const hostRow = AndroidDevice.fromPartial({
      id: 'shared-id',
      omnilabSpec: {
        labels: {
          fc_machine_type: { values: ['host'] },
          state: { values: ['LAB_MISSING'] },
        },
      },
    });

    queryClient.setQueryData<ListAndroidDevicesResponse>(
      [...LIST_QUERY_KEY, 'cached'],
      ListAndroidDevicesResponse.fromPartial({
        devices: [deviceRow, hostRow],
      }),
    );

    const { result } = renderHook(
      () => useAndroidDeviceData('shared-id', 'Android', 'host'),
      { wrapper },
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.device).toEqual(hostRow);
  });

  it('scopes filter and cache lookup to Pixel workspace with machineType', () => {
    renderHook(() => useAndroidDeviceData('pixel-shared', 'Pixel', 'host'), {
      wrapper,
    });

    expect(
      useAndroidDevicesModule.useListAndroidDevicesQueryKey,
    ).toHaveBeenCalledWith(undefined, 'Pixel');
    const request = useAndroidDevicesSpy.mock
      .calls[0][0] as ListAndroidDevicesRequest;
    expect(request.filter).toContain('"host_group" = "pte_labs"');
    expect(request.filter).toContain('id = "pixel-shared"');
    expect(request.filter).toContain('fc_machine_type = "host"');
  });
});
