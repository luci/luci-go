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

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { obtainAuthState } from '../common/api/auth_state';
import { trackedRedirect } from '../generic_libs/tools/react_router_utils/route_utils';

import {
  FLEET_CONSOLE_BASE_URL,
  generateDeviceDetailsURL,
  generateDeviceListURL,
  generateRepairsURL,
  REQUESTS_SUBROUTE,
} from './constants/paths';
import { platformRoutes } from './routing/platform_routes';

// IMPORTANT:
// When adding new routes, ensure that the path param does not contain PII.
// If you need PII in the path param, document it and scrub the URL param from
// GA4 tracking. See http://go/ooga-config#scrub-urls.
// The return type of a dynamic import.
type LazyModule = Promise<{ readonly [s: string]: unknown }>;

const loadNotFoundRoute = () => import('@/fleet/pages/not_found_page');

/**
 * A higher-order function that returns a lazy-loading function for a route.
 * The returned function only loads the route if the user is a Googler.
 * Otherwise, it loads a "not found" page or a custom fallback.
 */
export const loadRouteForGooglersOnly = (
  onSuccess: () => LazyModule,
  onFail: () => LazyModule = loadNotFoundRoute,
) => {
  return async () => {
    const isGoogler = await obtainAuthState()
      .then((s) => s.email?.endsWith('@google.com') ?? false)
      .catch(() => false);
    if (isGoogler) {
      return onSuccess();
    }
    return onFail();
  };
};

export type DevicesListHandle = {
  platform: Platform;
};

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
            path: 'p/:platform',
            children: platformRoutes,
          },
          {
            path: REQUESTS_SUBROUTE,
            children: [
              {
                index: true,
                lazy: () =>
                  import('@/fleet/pages/resource_request_insights_page'),
              },
            ],
          },
          {
            path: 'metrics',
            lazy: loadRouteForGooglersOnly(
              () => import('@/fleet/pages/metrics_page'),
            ),
          },
          {
            path: 'planners',
            lazy: loadRouteForGooglersOnly(
              () => import('@/fleet/pages/resource_planner_insights_page'),
            ),
          },
          { path: 'sandbox', lazy: () => import('@/fleet/pages/sandbox_page') },

          // Redirects we added for locations where pages used to live.
          {
            path: 'devices/:id?',
            loader: ({ params }) =>
              trackedRedirect({
                contentGroup: 'fleet',
                from: `${FLEET_CONSOLE_BASE_URL}/devices${
                  params.id ? `/${params.id}` : ''
                }`,
                to: params.id
                  ? generateDeviceDetailsURL('chromeos', params.id)
                  : generateDeviceListURL('chromeos'),
              }),
          },
          {
            path: 'repairs/:platform',
            loader: ({ params }) =>
              trackedRedirect({
                contentGroup: 'fleet',
                from: `${FLEET_CONSOLE_BASE_URL}/repairs/${params.platform}`,
                to: generateRepairsURL(params.platform || 'android'),
              }),
          },
          // End redirects
        ],
      },
      {
        path: '*',
        lazy: loadNotFoundRoute,
      },
    ],
  },
];
