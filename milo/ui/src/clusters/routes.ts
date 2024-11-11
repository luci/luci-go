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

import { RouteObject } from 'react-router-dom';

// IMPORTANT:
// When adding new routes, ensure that the path param does not contain PII.
// If you need PII in the path param, document it and scrub the URL param from
// GA4 tracking. See http://go/ooga-config#scrub-urls.
export const clustersRoutes: RouteObject[] = [
  {
    path: 'labs',
    lazy: () => import('@/common/layouts/labs_layout'),
    children: [
      {
        lazy: () => import('@/clusters/pages/base'),
        children: [
          {
            path: 'help',
            lazy: () => import('@/clusters/pages/help/help'),
          },
          {
            path: 'b/:bugTracker/:id',
            lazy: () => import('@/clusters/pages/bug/bug'),
          },
          {
            path: 'b/:id',
            lazy: () => import('@/clusters/pages/bug/bug'),
          },
          {
            path: 'p/:project',
            children: [
              {
                path: 'rules',
                children: [
                  {
                    index: true,
                    lazy: () => import('@/clusters/pages/rules/rules'),
                  },
                  {
                    path: 'new',
                    lazy: () => import('@/clusters/pages/new_rule/new_rule'),
                  },
                  {
                    path: ':id',
                    lazy: () => import('@/clusters/pages/rule/rule'),
                  },
                ],
              },
              {
                path: 'clusters',
                children: [
                  {
                    index: true,
                    lazy: () => import('@/clusters/pages/clusters/clusters'),
                  },
                  {
                    path: ':algorithm/:id',
                    lazy: () =>
                      import('@/clusters/pages/clusters/cluster/cluster'),
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
];
