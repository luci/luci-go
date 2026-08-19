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

import { lazy, ReactElement } from 'react';
import { Navigate, RouteObject } from 'react-router';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getFeatureFlag } from '../config/features';
import { initiateSurvey } from '../utils/survey';

import { PlatformDependentPage } from './platform_dependent_page';
export type PageComponentMap = Partial<
  Record<Platform, React.ComponentType | ReactElement>
>;

const AndroidDevicesPage = lazy(
  () => import('@/fleet/pages/device_list_page/android'),
);

const AndroidDeviceDetailsPage = lazy(() =>
  import(
    '@/fleet/pages/device_details_page/android/android_device_details_page'
  ).then((module) => ({
    default: module.Component,
  })),
);

export const platformRoutes: RouteObject[] = [
  {
    path: '',
    element: (
      <PlatformDependentPage
        pageComponentMap={{
          [Platform.ANDROID]: <Navigate to={'repairs'} />,
          [Platform.PIXEL]: <Navigate to={'devices'} />,
          [Platform.CHROMEOS]: <Navigate to={'devices'} />,
        }}
      />
    ),
  },

  {
    path: 'repairs',
    element: (
      <PlatformDependentPage
        pageComponentMap={{
          [Platform.ANDROID]: lazy(
            () => import('@/fleet/pages/android/repairs'),
          ),
          ...(getFeatureFlag('ChromeOsRepairsDashboard')
            ? {
                [Platform.CHROMEOS]: lazy(
                  () => import('@/fleet/pages/chromeos/repairs'),
                ),
              }
            : {}),
        }}
      />
    ),
  },

  {
    path: 'devices',
    loader: async () => {
      // Fire-and-forget. Don't block the page load.
      initiateSurvey(SETTINGS.fleetConsole.hats);
      return null;
    },

    children: [
      {
        index: true,
        element: (
          <PlatformDependentPage
            pageComponentMap={{
              [Platform.ANDROID]: <AndroidDevicesPage workspace="Android" />,
              [Platform.PIXEL]: <AndroidDevicesPage workspace="Pixel" />,
              [Platform.CHROMEOS]: lazy(
                () => import('@/fleet/pages/device_list_page/chromeos'),
              ),
              [Platform.CHROMIUM]: lazy(
                () => import('@/fleet/pages/device_list_page/browser'),
              ),
            }}
          />
        ),
      },
      {
        path: ':id',
        element: (
          <PlatformDependentPage
            pageComponentMap={{
              [Platform.CHROMEOS]: lazy(
                () => import('@/fleet/pages/device_details_page'),
              ),
              [Platform.ANDROID]: <AndroidDeviceDetailsPage />,
              [Platform.PIXEL]: <AndroidDeviceDetailsPage />,
              [Platform.CHROMIUM]: lazy(() =>
                import(
                  '@/fleet/pages/device_details_page/browser/browser_device_details_page'
                ).then((module) => ({
                  default: module.Component,
                })),
              ),
            }}
          />
        ),
      },
    ],
  },

  {
    path: 'admin-tasks',
    element: (
      <PlatformDependentPage
        pageComponentMap={{
          [Platform.CHROMEOS]: lazy(
            () => import('@/fleet/pages/admin_tasks_page'),
          ),
        }}
      />
    ),
  },
];
