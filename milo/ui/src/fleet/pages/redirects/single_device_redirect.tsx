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

import { Navigate } from 'react-router';

import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import {
  getFilters,
  stringifyFilters,
} from '@/fleet/components/multi_select_filter/search_param_utils/search_param_utils';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useDevices } from '@/fleet/hooks/use_devices';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ListDevicesRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export function SingleDeviceRedirect() {
  const [searchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const filter = stringifyFilters(getFilters(searchParams));

  const request = ListDevicesRequest.fromPartial({
    pageSize: 1,
    orderBy: orderByParam,
    filter,
  });

  const devicesQuery = useDevices(request);

  // Intentionally show a blank page when loading. Showing a loading bar is
  // more jarring because the load time is quick for one device.
  if (devicesQuery.isFetching) return <></>;

  // TODO: b/401486024 - Use shared error code for this.
  if (devicesQuery.error) {
    return (
      <AlertWithFeedback
        testId="single-device-redirect"
        severity="error"
        title="Redirection failed"
        bugErrorMessage={`Device not found for query: ${filter}`}
      >
        <p>An error occured.</p>
      </AlertWithFeedback>
    );
  }

  if (!devicesQuery.data?.devices.length) {
    return (
      <AlertWithFeedback
        testId="single-device-redirect"
        severity="error"
        title="No devices found"
        bugErrorMessage={`Device not found for query: ${filter}`}
      >
        <p>
          No devices matched the search, so the redirection was canceled. It
          {"'"}s possible the link you clicked might be malformed.
        </p>
      </AlertWithFeedback>
    );
  }

  return (
    <Navigate
      to={`/ui/fleet/labs/devices/${devicesQuery.data?.devices[0].id}`}
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
