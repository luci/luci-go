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

import { Router } from '@vaadin/router';
import { BroadcastChannel } from 'broadcast-channel';

import './components/page_layout';
import { refreshAuthChannel } from './components/page_layout';

export const NOT_FOUND_URL = '/not-found';

const appRoot = document.getElementById('app-root');
export const router = new Router(appRoot, { baseUrl: '/ui/' });
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
          './pages/login_page'
        );
        return cmd.component('milo-login-page');
      },
    },
    {
      path: '/search',
      name: 'search',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "search_page" */
          './pages/search_page'
        );
        return cmd.component('milo-search-page');
      },
    },
    {
      path: '/inv/:invocation_id',
      name: 'invocation',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "invocation_page" */
          './pages/invocation_page'
        );
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
              './pages/test_results_tab'
            );
            return cmd.component('milo-test-results-tab');
          },
        },
        {
          path: '/invocation-details',
          name: 'invocation-details',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "invocation_details_tab" */
              './pages/invocation_page/invocation_details_tab'
            );
            return cmd.component('milo-invocation-details-tab');
          },
        },
      ],
    },
    {
      path: '/p/:project/builders',
      name: 'project-builders',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "builders_page" */
          './pages/builders_page'
        );
        return cmd.component('milo-builders-page');
      },
    },
    {
      path: '/p/:project/g/:group/builders',
      name: 'group-builders',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "builders_page" */
          './pages/builders_page'
        );
        return cmd.component('milo-builders-page');
      },
    },
    {
      path: '/b/:build_id/:path*',
      name: 'build-short-link',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "build_page" */
          './pages/build_page'
        );
        return cmd.component('milo-build-page');
      },
    },
    {
      path: '/p/:project/builders/:bucket/:builder/:build_num_or_id',
      name: 'build',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "build_page" */
          './pages/build_page'
        );
        return cmd.component('milo-build-page');
      },
      children: [
        {
          path: '/',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "build_default_tab" */
              './pages/build_page/build_default_tab'
            );
            return cmd.component('milo-build-default-tab');
          },
        },
        {
          path: '/overview',
          name: 'build-overview',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "overview_tab" */
              './pages/build_page/overview_tab'
            );
            return cmd.component('milo-overview-tab');
          },
        },
        {
          path: '/test-results',
          name: 'build-test-results',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "test_results_tab" */
              './pages/test_results_tab'
            );
            return cmd.component('milo-test-results-tab');
          },
        },
        {
          path: '/steps',
          name: 'build-steps',
          children: [
            {
              path: '/',
              action: async (_ctx, cmd) => {
                await import(
                  /* webpackChunkName: "steps_tab" */
                  './pages/build_page/steps_tab'
                );
                return cmd.component('milo-steps-tab');
              },
            },
            {
              // Some old systems generate links to a step by appending suffix
              // to /steps/ (crbug/1204954).
              // This allows those links to continue to work.
              path: '/:path+',
              redirect: '/p/:project/builders/:bucket/:builder/:build_num_or_id/steps',
            },
          ],
        },
        {
          path: '/related-builds',
          name: 'build-related-builds',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "related_builds_tab" */
              './pages/build_page/related_builds_tab'
            );
            return cmd.component('milo-related-builds-tab');
          },
        },
        {
          path: '/timeline',
          name: 'build-timeline',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "timeline_tab" */
              './pages/build_page/timeline_tab'
            );
            return cmd.component('milo-timeline-tab');
          },
        },
        {
          path: '/blamelist',
          name: 'build-blamelist',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "blamelist_tab" */
              './pages/build_page/blamelist_tab'
            );
            return cmd.component('milo-blamelist-tab');
          },
        },
      ],
    },
    {
      path: '/artifact',
      name: 'artifact',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "artifact_page" */
          './pages/artifact/artifact_page_layout'
        );
        return cmd.component('milo-artifact-page-layout');
      },
      children: [
        {
          path: '/text-diff/invocations/:inv_id/(tests)?/:test_id?/(results)?/:result_id?/artifacts/:artifact_id',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "text_diff_artifact_page" */
              './pages/artifact/text_diff_artifact_page'
            );
            return cmd.component('milo-text-diff-artifact-page');
          },
        },
        {
          path: '/image-diff/invocations/:inv_id/(tests)?/:test_id?/(results)?/:result_id?/artifacts/:artifact_id',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "image_diff_artifact_page" */
              './pages/artifact/image_diff_artifact_page'
            );
            return cmd.component('milo-image-diff-artifact-page');
          },
        },
        {
          path: '/raw/invocations/:inv_id/(tests)?/:test_id?/(results)?/:result_id?/artifacts/:artifact_id',
          action: async (_ctx, cmd) => {
            await import(
              /* webpackChunkName: "raw_artifact_page" */
              './pages/artifact/raw_artifact_page'
            );
            return cmd.component('milo-raw-artifact-page');
          },
        },
      ],
    },
    {
      path: '/test/:realm/:test_id',
      name: 'test-history',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "test_history_page" */
          './pages/test_history_page'
        );
        return cmd.component('milo-test-history-page');
      },
    },
    {
      path: '/auth-callback/:channel_id',
      action: async (ctx, cmd) => {
        new BroadcastChannel(ctx.params['channel_id'] as string).postMessage('close');
        refreshAuthChannel.postMessage('refresh');
        await import(
          /* webpackChunkName: "close_page" */
          './pages/close_page'
        );
        return cmd.component('milo-close-page');
      },
    },
    {
      path: '/:path*',
      action: async (_ctx, cmd) => {
        await import(
          /* webpackChunkName: "not_found_page" */
          './pages/not_found_page'
        );
        return cmd.component('milo-not-found-page');
      },
    },
  ],
});

/**
 * Generates the URL to a raw artifact page.
 */
export function getRawArtifactUrl(artifactName: string) {
  return router.urlForName('artifact') + '/raw/' + artifactName;
}
