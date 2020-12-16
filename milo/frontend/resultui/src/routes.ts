// Copyright 2020 The LUCI Authors.
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

import { Route, Router } from '@vaadin/router';

import './components/page_layout';
import './context/build_state/build_state_provider';
import './context/invocation_state/invocation_state_provider';

const notFoundRoute: Route = {
  path: '/:path*',
  action: async (_ctx, cmd) => {
    await import(
      /* webpackChunkName: "not_found_page" */
      /* webpackPrefetch: true */
      './pages/not_found_page');
    return cmd.component('milo-not-found-page');
  },
};

export const NOT_FOUND_URL = '/ui/not-found';

const appRoot = document.getElementById('app-root');
export const router = new Router(appRoot, {baseUrl: '/ui/'});
router.setRoutes({
  path: '/',
  component: 'milo-page-layout',
  children: [
    {
      path: '/login',
      name: 'login',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "login_page" */
          /* webpackPrefetch: true */
          './pages/login_page');
        return cmd.component('milo-login-page');
      },
    },
    {
      path: '/error',
      name: 'error',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "error_page" */
          /* webpackPrefetch: true */
          './pages/error_page');
        return cmd.component('milo-error-page');
      },
    },
    {
      path: '/',
      component: 'milo-invocation-state-provider',
      children: [
        {
          path: '/inv/:invocation_id',
          name: 'invocation',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "invocation_page" */
              /* webpackPrefetch: true */
              './pages/invocation_page');
            return cmd.component('milo-invocation-page');
          },
          children: [
            {
              path: '/',
              redirect: '/inv/:invocation_id/test-results',
            },
            {
              path: '/test-results',
              name: 'invocation-test-results',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "test_results_tab" */
                  /* webpackPrefetch: true */
                  './pages/test_results_tab');
                return cmd.component('milo-test-results-tab');
              },
            },
            {
              path: '/invocation-details',
              name: 'invocation-details',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "invocation_details_tab" */
                  /* webpackPrefetch: true */
                  './pages/invocation_page/invocation_details_tab');
                return cmd.component('milo-invocation-details-tab');
              },
            },
          ],
        },
        {
          path: '/p/:project/builders/:bucket/:builder/:build_num_or_id',
          component: 'milo-build-state-provider',
          children: [
            {
              path: '/',
              name: 'build',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "build_page" */
                  /* webpackPrefetch: true */
                  './pages/build_page');
                return cmd.component('milo-build-page');
              },
              children: [
                {
                  path: '/',
                  redirect: '/p/:project/builders/:bucket/:builder/:build_num_or_id/overview',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "build_default_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/build_default_tab');
                    return cmd.component('milo-build-default-tab');
                  },
                },
                {
                  path: '/overview',
                  name: 'build-overview',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "overview_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/overview_tab');
                    return cmd.component('milo-overview-tab');
                  },
                },
                {
                  path: '/test-results',
                  name: 'build-test-results',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "test_results_tab" */
                      /* webpackPrefetch: true */
                      './pages/test_results_tab');
                    return cmd.component('milo-test-results-tab');
                  },
                },
                {
                  path: '/steps',
                  name: 'build-steps',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "steps_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/steps_tab');
                    return cmd.component('milo-steps-tab');
                  },
                },
                {
                  path: '/related-builds',
                  name: 'build-related-builds',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "related_builds_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/related_builds_tab');
                    return cmd.component('milo-related-builds-tab');
                  },
                },
                {
                  path: '/timeline',
                  name: 'build-timeline',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "timeline_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/timeline_tab');
                    return cmd.component('milo-timeline-tab');
                  },
                },
                {
                  path: '/blamelist',
                  name: 'build-blamelist',
                  action: async (_ctx, cmd) => {
                    await import(
                      /* webpackChunkName: "blamelist_tab" */
                      /* webpackPrefetch: true */
                      './pages/build_page/blamelist_tab');
                    return cmd.component('milo-blamelist-tab');
                  },
                },
              ],
            },
          ],
        },
        {
          path: '/artifact',
          children: [
            {
              path: '/text-diff/:artifact_name',
              name: 'text-diff-artifact',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "text_diff_artifact_page" */
                  /* webpackPrefetch: true */
                  './pages/artifact/text_diff_artifact_page');
                return cmd.component('milo-text-diff-artifact-page');
              },
            },
            {
              path: '/image-diff/:artifact_name',
              name: 'image-diff-artifact',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "image_diff_artifact_page" */
                  /* webpackPrefetch: true */
                  './pages/artifact/image_diff_artifact_page');
                return cmd.component('milo-image-diff-artifact-page');
              },
            },
          ],
        },
      ],
    },
    notFoundRoute,
  ],
});

appRoot?.addEventListener('error', (event) => {
  const searchParams = new URLSearchParams({
    reason: event.message,
    sourceUrl: window.location.toString(),
  });
  Router.go({
    pathname: router.urlForName('error'),
    search: '?' + searchParams.toString(),
  });
});
