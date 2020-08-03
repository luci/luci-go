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
import './context/app_state/app_state_provider';
import './context/build_state/build_state_provider';
import './context/config_provider';
import './context/invocation_state/invocation_state_provider';

const notFoundRoute: Route = {
  path: '/:path*',
  action: async (_ctx, cmd) => {
    await import(/* webpackChunkName: "not_found_page" */ './pages/not_found_page');
    return cmd.component('tr-not-found-page');
  },
};

export const NOT_FOUND_URL = '/ui/not-found';

const appRoot = document.getElementById('app-root');
export const router = new Router(appRoot);
router.setRoutes({
  path: '/ui',
  component: 'tr-app-config-provider',
  children: [
    {
      path: '/',
      component: 'tr-page-layout',
      children: [
        {
          path: '/login',
          name: 'login',
          action: async (_ctx, cmd) => {
            await import(/* webpackChunkName: "login_page" */ './pages/login_page');
            return cmd.component('tr-login-page');
          },
        },
        {
          path: '/error',
          name: 'error',
          action: async (_ctx, cmd) => {
            await import(/* webpackChunkName: "error_page" */ './pages/error_page');
            return cmd.component('tr-error-page');
          },
        },
        {
          path: '/',
          component: 'tr-app-state-provider',
          children: [
            {
              path: '/',
              component: 'tr-invocation-state-provider',
              children: [
                {
                  path: '/inv/:invocation_id',
                  name: 'invocation',
                  action: async (_ctx, cmd) => {
                    await import(/* webpackChunkName: "invocation_page" */ './pages/invocation_page');
                    return cmd.component('tr-invocation-page');
                  },
                  children: [
                    {
                      path: '/',
                      redirect: '/ui/inv/:invocation_id/test-results',
                    },
                    {
                      path: 'test-results',
                      name: 'invocation-test-results',
                      action: async (_ctx, cmd) => {
                        await import(/* webpackChunkName: "test_results_tab" */ './pages/test_results_tab');
                        return cmd.component('tr-test-results-tab');
                      },
                    },
                    {
                      path: '/invocation-details',
                      name: 'invocation-details',
                      action: async (_ctx, cmd) => {
                        await import(/* webpackChunkName: "invocation_details_tab" */ './pages/invocation_page/invocation_details_tab');
                        return cmd.component('tr-invocation-details-tab');
                      },
                    },
                    notFoundRoute,
                  ],
                },
                {
                  path: '/p/:project/:bucket/:builder/:build_num_or_id',
                  component: 'tr-build-state-provider',
                  children: [
                    {
                      path: '/',
                      name: 'build',
                      action: async (_ctx, cmd) => {
                        await import(/* webpackChunkName: "build_page" */ './pages/build_page');
                        return cmd.component('tr-build-page');
                      },
                      children: [
                        {
                          path: '/',
                          redirect: '/ui/p/:project/:bucket/:builder/:build_num_or_id/test-results',
                        },
                        {
                          path: '/test-results',
                          name: 'build-test-results',
                          action: async (_ctx, cmd) => {
                            await import(/* webpackChunkName: "test_results_tab" */ './pages/test_results_tab');
                            return cmd.component('tr-test-results-tab');
                          },
                        },
                        {
                          path: '/related-builds',
                          name: 'build-related-builds',
                          action: async (_ctx, cmd) => {
                            await import(/* webpackChunkName: "related_builds_tab" */ './pages/related_builds_tab');
                            return cmd.component('tr-related-builds-tab');
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
                        await import(/* webpackChunkName: "text_diff_artifact_page" */ './pages/artifact/text_diff_artifact_page');
                        return cmd.component('tr-text-diff-artifact-page');
                      },
                    },
                    {
                      path: '/image-diff/:artifact_name',
                      name: 'image-diff-artifact',
                      action: async (_ctx, cmd) => {
                        await import(/* webpackChunkName: "image_diff_artifact_page" */ './pages/artifact/image_diff_artifact_page');
                        return cmd.component('tr-image-diff-artifact-page');
                      },
                    },
                    notFoundRoute,
                  ],
                },
                notFoundRoute,
              ],
            },
          ],
        },
        notFoundRoute,
      ],
    },
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
