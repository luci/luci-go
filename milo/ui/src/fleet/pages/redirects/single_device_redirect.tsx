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

import { getFilterValue } from '@/fleet/components/filter_dropdown/search_param_utils';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useDevices } from '@/fleet/hooks/use_devices';
import { BaseDeviceRedirect } from '@/fleet/pages/redirects/base_device_redirect';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ListDevicesRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export function SingleDeviceRedirect() {
  const [searchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const filter = getFilterValue(searchParams);

  const request = ListDevicesRequest.fromPartial({
    pageSize: 1,
    orderBy: orderByParam,
    filter,
  });

  const devicesQuery = useDevices(request);

  return (
    <BaseDeviceRedirect
      isLoading={devicesQuery.isFetching}
      error={devicesQuery.error}
      devices={devicesQuery.data?.devices}
      generateUrl={generateChromeOsDeviceDetailsURL}
      testId="single-device-redirect"
      filter={filter}
    />
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-single-device-redirect">
      <SingleDeviceRedirect />
    </TrackLeafRoutePageView>
  );
}
