// Copyright 2023 The LUCI Authors.
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

// We cannot use module alias (e.g. `@/<package>`) here because they are not
// not supported in vite.config.ts. And we need to import those routes in
// vite.config.ts to compute a regex at build time.

import { authRoutes } from '../../authdb/routes';
import { clustersRoutes } from '../../clusters/routes';
import { fleetRoutes } from '../../fleet/routes';
import { swarmingRoutes } from '../../swarming/routes';
import { treeStatusRoutes } from '../../tree_status/routes';

// IMPORTANT:
// When adding new routes, ensure that the path param does not contain PII.
// If you need PII in the path param, document it and scrub the URL param from
// GA4 tracking. See http://go/ooga-config#scrub-urls.
//
// New routes under a feature area should be added to `@/<feature-area>/routes`.
// See go/luci-ui-path-segregation for details.
export const routes: RouteObject[] = [
  {
    index: true,
    lazy: () => import('@/core/pages/project_search_page'),
  },
  {
    path: 'login',
    lazy: () => import('@/core/pages/login_page'),
  },
  {
    path: 'local-login-instruction',
    lazy: () => import('@/core/pages/local_login_instruction_page'),
  },
  {
    path: 'search',
    lazy: () => import('@/core/routes/search_loader/search_redirection_loader'),
  },
  {
    path: 'internal/always-fail',
    lazy: () => import('@/core/pages/always_fail_page'),
  },
  {
    path: 'internal/gallery',
    lazy: () => import('@/core/pages/gallery_page'),
  },
  {
    path: 'doc/release-notes',
    lazy: () => import('@/core/pages/release_notes_page'),
  },
  {
    path: 'builder-search',
    lazy: () => import('@/build/pages/builder_search_page'),
  },
  {
    path: 'p/:project/test-search',
    lazy: () => import('@/test_verdict/pages/test_search'),
  },
  {
    path: 'p/:project',
    lazy: () => import('@/build/pages/console_list_page'),
  },
  {
    path: 'p/:project/builders',
    lazy: () => import('@/build/pages/builder_list_page'),
  },
  {
    path: 'p/:project/g/:group/builders',
    lazy: () => import('@/build/pages/builder_group_page'),
  },
  {
    path: 'p/:project/builders/:bucket/:builder',
    lazy: () => import('@/build/pages/builder_page'),
  },
  {
    path: 'b/:buildId',
    children: [
      {
        path: '*?',
        lazy: () => import('@/build/legacy/build_page/build_page_short_link'),
      },
    ],
  },
  {
    path: 'p/:project/builders/:bucket/:builder/:buildNumOrId',
    lazy: () => import('@/build/legacy/build_page'),
    children: [
      {
        index: true,
        lazy: () => import('@/build/legacy/build_page/build_default_tab'),
      },
      {
        path: 'overview',
        lazy: () => import('@/build/legacy/build_page/overview_tab'),
      },
      {
        path: 'infra',
        lazy: () => import('@/build/legacy/build_page/infra_tab'),
      },
      {
        path: 'test-results',
        lazy: () => import('@/test_verdict/legacy/test_results_tab'),
      },
      {
        path: 'related-builds',
        lazy: () => import('@/build/legacy/build_page/related_builds_tab'),
      },
      {
        path: 'timeline',
        lazy: () => import('@/build/legacy/build_page/timeline_tab'),
      },
      {
        path: 'blamelist',
        lazy: () => import('@/build/legacy/build_page/blamelist_tab'),
      },
    ],
  },
  {
    path: 'inv/:invId',
    lazy: () => import('@/test_verdict/legacy/invocation_page'),
    children: [
      {
        index: true,
        lazy: () =>
          import(
            '@/test_verdict/legacy/invocation_page/invocation_default_tab'
          ),
      },
      {
        path: 'test-results',
        lazy: () => import('@/test_verdict/legacy/test_results_tab'),
      },
      {
        path: 'invocation-details',
        lazy: () => import('@/test_verdict/pages/invocation_page/details_tab'),
      },
    ],
  },
  {
    path: 'artifact',
    lazy: () => import('@/test_verdict/legacy/artifact/artifact_page_layout'),
    children: [
      {
        path: 'text-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/text_diff_artifact_page'),
      },
      {
        path: 'text-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/text_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/image_diff_artifact_page'),
      },
      {
        path: 'image-diff/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () =>
          import('@/test_verdict/legacy/artifact/image_diff_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/artifacts/:artifactId',
        lazy: () => import('@/test_verdict/legacy/artifact/raw_artifact_page'),
      },
      {
        path: 'raw/invocations/:invId/tests/:testId/results/:resultId/artifacts/:artifactId',
        lazy: () => import('@/test_verdict/legacy/artifact/raw_artifact_page'),
      },
    ],
  },
  {
    path: 'test/:projectOrRealm/:testId',
    lazy: () => import('@/test_verdict/legacy/test_history_page'),
  },
  {
    path: 'bisection',
    children: [
      {
        path: '*',
        lazy: () => import('@/bisection/pages/redirection_loader'),
        // index: true,
      },
    ],
  },
  {
    path: 'p/:project/bisection',
    lazy: () => import('@/bisection/layouts/base'),
    children: [
      {
        path: '',
        lazy: () => import('@/bisection/pages/analyses_page'),
        children: [
          {
            index: true,
            lazy: () => import('@/bisection/pages/analyses_default_tab'),
          },
          {
            path: 'compile-analysis',
            lazy: () => import('@/bisection/pages/compile_analyses_tab'),
          },
          {
            path: 'test-analysis',
            lazy: () => import('@/bisection/pages/test_analyses_tab'),
          },
        ],
      },
      {
        path: 'compile-analysis/b/:bbid',
        lazy: () => import('@/bisection/pages/analysis_details'),
      },
      {
        path: 'test-analysis/b/:id',
        lazy: () => import('@/bisection/pages/test_analysis_details'),
      },
    ],
  },
  {
    path: 'monitoring',
    lazy: () => import('@/monitoringv2/pages/monitoring_page'),
  },
  {
    path: 'monitoring/:tree',
    lazy: () => import('@/monitoringv2/pages/monitoring_page'),
  },
  // When promoting lab pages out of labs, move them to their respective feature
  // area. See go/luci-ui-path-segregation for details.
  // New lab pages should be mounted to `/ui/<feature>/labs/my-new-lab-page`
  // instead.
  // TODO: remove this comment once all existing lab routes are removed.
  {
    path: 'labs',
    lazy: () => import('@/common/layouts/labs_layout'),
    children: [
      {
        path: 'monitoring',
        lazy: () => import('@/monitoring/pages/monitoring_page'),
      },
      {
        path: 'monitoring/:tree',
        lazy: () => import('@/monitoring/pages/monitoring_page'),
      },
      {
        path: 'monitoringv2',
        lazy: () => import('@/monitoringv2/pages/redirection_loader'),
      },
      {
        path: 'monitoringv2/:tree',
        lazy: () => import('@/monitoringv2/pages/redirection_loader'),
      },
      {
        // TODO: Remove this endpoint after we remove all client.
        path: 'tree-status/:tree',
        lazy: () => import('@/tree_status/pages/redirection_loader'),
      },
      {
        path: 'p/:project/regressions',
        lazy: () => import('@/test_verdict/pages/recent_regressions_page'),
      },
      {
        // TODO(b/321110247): once we have a stable regression group ID, replace
        // "details" with the regression group ID.
        path: 'p/:project/regressions/details',
        lazy: () => import('@/test_verdict/pages/regression_details_page'),
      },
      {
        path: 'p/:project/tests/:testId/variants/:variantHash/refs/:refHash/blamelist',
        lazy: () => import('@/test_verdict/pages/blamelist_page'),
      },
      {
        path: 'p/:project/log-search',
        lazy: () => import('@/test_verdict/pages/log_search_page'),
        children: [
          {
            index: true,
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/log_default_tab'),
          },
          {
            path: 'test-logs',
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/test_logs_tab'),
          },
          {
            path: 'shared-logs',
            lazy: () =>
              import('@/test_verdict/pages/log_search_page/shared_logs_tab'),
          },
        ],
      },
      {
        path: 'inv/:invId',
        lazy: () => import('@/test_verdict/pages/invocation_page'),
        children: [
          {
            path: 'invocation-details',
            lazy: () =>
              import('@/test_verdict/pages/invocation_page/details_tab'),
          },
        ],
      },
    ],
  },
  {
    path: 'test-investigate',
    // TODO: Add child for new URL structure:
    // inv/:inv-id/module/:module-name/scheme/:module-scheme/variant/:variant/coarse/:coarse-name/fine/:fine-name/test-id/:test-id
    children: [
      {
        path: 'invocations/:invocationId',
        lazy: () => import('@/test_investigation/pages/invocation_page'),
      },
      {
        path: 'invocations/:invocationId/tests/:testId/variants/:variantHash',
        lazy: () => import('@/test_investigation/pages/test_investigate_page'),
      },
    ],
  },
  {
    path: 'fleet',
    children: fleetRoutes,
  },
  {
    path: 'swarming',
    children: swarmingRoutes,
  },
  {
    path: 'tree-status',
    children: treeStatusRoutes,
  },
  {
    path: 'tests',
    children: clustersRoutes,
  },
  {
    path: 'auth',
    children: authRoutes,
  },
];
