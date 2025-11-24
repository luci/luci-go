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

import { RouteObject } from 'react-router';

import { redirectToChildPath } from '@/generic_libs/tools/react_router_utils';

export const chronicleRoutes: RouteObject[] = [
  {
    path: ':workplanId',
    lazy: () => import('./pages/chronicle_page'),
    children: [
      {
        index: true,
        loader: redirectToChildPath('summary'),
      },
      {
        path: 'summary',
        lazy: () => import('./components/summary_view'),
      },
      {
        path: 'timeline',
        lazy: () => import('./components/timeline_view'),
      },
      {
        path: 'graph',
        lazy: () => import('./components/graph_view'),
      },
      {
        path: 'tree',
        lazy: () => import('./components/tree_view/tree_view'),
      },
      {
        path: 'ledger',
        lazy: () => import('./components/ledger_view'),
      },
    ],
  },
];
