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

import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

/**
 * Returns the Mobile Harness URL for a given Android device or host.
 *
 * For host machines:
 * - If hostname and host_ip are available, returns the lab detail view.
 * - Otherwise, returns the lab list view filtered by device ID.
 *
 * For individual devices:
 * - If hostname and host_ip are available, returns the device detail view.
 * - Otherwise, returns the device list view filtered by device ID.
 */
export const getMobileHarnessLink = (device?: AndroidDevice | null): string => {
  if (!device) return '';

  const type = device.omnilabSpec?.labels?.['fc_machine_type']?.values?.[0];
  const hostname =
    device.omnilabSpec?.labels?.['hostname']?.values?.[0] ||
    device.omnilabSpec?.labels?.['host_name']?.values?.[0];
  const hostIp = device.omnilabSpec?.labels?.['host_ip']?.values?.[0];

  if (type === 'host') {
    if (hostname && hostIp) {
      return `https://mobileharness-fe.corp.google.com/labdetailview/${hostname}/${hostIp}`;
    }
    const urlParams = new URLSearchParams();
    urlParams.append('filter', `"host_name":("${device.id}")`);
    return `https://mobileharness-fe.corp.google.com/lablistview?${urlParams.toString()}`;
  }
  if (hostname && hostIp) {
    return `https://mobileharness-fe.corp.google.com/devicedetailview/${hostname}/${hostIp}/${device.id}`;
  }
  const urlParams = new URLSearchParams();
  urlParams.append('filter', `"id":("${device.id}")`);
  return `https://mobileharness-fe.corp.google.com/devicelistview?${urlParams.toString()}`;
};
