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

import type { RouteObject } from 'react-router';

// IMPORTANT:
// When adding new routes, ensure that the path param does not contain PII.
// If you need PII in the path param, document it and scrub the URL param from
// GA4 tracking. See http://go/ooga-config#scrub-urls.
export const fleetRoutes: RouteObject[] = [
  {
    path: '',
    lazy: () => import('@/fleet/root'),
    children: [
      {
        path: 'redirects',
        children: [
          {
            path: 'singledevice',
            index: true,
            lazy: () =>
              import('@/fleet/pages/redirects/single_device_redirect'),
          },
          {
            path: 'swarming',
            children: [
              {
                path: '*',
                lazy: () => import('@/fleet/pages/redirects/swarming_redirect'),
              },
            ],
          },
        ],
      },
      {
        path: 'labs',
        lazy: () => import('@/fleet/layouts/fleet_labs_layout'),
        children: [
          {
            path: 'devices',
            children: [
              {
                index: true,
                lazy: () => import('@/fleet/pages/device_list_page'),
              },
              {
                path: ':id',
                lazy: () => import('@/fleet/pages/device_details_page'),
              },
            ],
          },
          {
            path: 'requests',
            children: [
              {
                index: true,
                lazy: () =>
                  import('@/fleet/pages/resource_request_insights_page'),
              },
            ],
          },
          { path: 'sandbox', lazy: () => import('@/fleet/pages/sandbox_page') },
        ],
      },
      {
        path: '*',
        lazy: () => import('@/fleet/pages/not_found_page'),
      },
    ],
  },
  // Prototype of a new unified UI for fleet management.
  // See: go/streamline-fleet-UI
];
