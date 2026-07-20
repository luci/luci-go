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

import { redirect, RouteObject } from 'react-router';

import { redirectToChildPath } from '@/generic_libs/tools/react_router_utils';

import { fromString, root, toString as idToString } from './utils/id';

export const chronicleRoutes: RouteObject[] = [
  // Special redirects to support short links with workplan ID at the end.
  ...['summary', 'timeline', 'graph', 'tree', 'ledger'].map(
    (view): RouteObject => ({
      path: `${view}/:workplanId`,
      loader: ({ params, request }) => {
        const url = new URL(request.url);
        return redirect(
          `/ui/chronicle/${params.workplanId}/${view}${url.search}${url.hash}`,
        );
      },
    }),
  ),

  // Canonical routes
  {
    path: ':workplanId',
    loader: ({ params, request }) => {
      const { workplanId } = params;
      if (workplanId) {
        try {
          // If this is a node ID, we need to parse it apart and reformat to
          // match our canonical URL format. This is used for supporting the
          // Go-links written into our Monarch metrics.
          const parsed = fromString(workplanId);
          const resolved = root(parsed);
          const realWorkplanId = resolved.wp?.id;
          if (realWorkplanId && realWorkplanId !== workplanId) {
            const url = new URL(request.url);
            const decodedPath = decodeURIComponent(url.pathname);
            url.pathname = decodedPath.replace(workplanId, realWorkplanId);
            const localNodeId = idToString(parsed, { omitWorkPlan: true });
            if (localNodeId) {
              url.searchParams.set('nodeId', localNodeId);
            }
            return redirect(url.pathname + url.search + url.hash);
          }
        } catch {
          // If parsing fails, it's not a node ID, ignore and fall through
          // to handling it as a workplan ID.
        }
      }
      return null;
    },
    lazy: () => import('./pages/chronicle_page'),
    children: [
      {
        index: true,
        loader: redirectToChildPath('graph'),
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
