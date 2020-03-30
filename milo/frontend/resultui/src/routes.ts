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

const router = new Router(document.getElementById('app-root'));
router.setRoutes([
  {
    path: '/invocation/:invocation_name',
    action: async (_ctx, cmd) => {
      await import(/* webpackChunkName: "invocation_page" */ './pages/invocation_page');
      return cmd.component('tr-invocation-page');
    },
  },
  {
    path: '/login',
    action: async (_ctx, cmd) => {
      await import(/* webpackChunkName: "login_page" */ './pages/login_page');
      return cmd.component('tr-login-page');
    },
  },
  {
    path: '/error',
    action: async (_ctx, cmd) => {
      await import(/* webpackChunkName: "error_page" */ './pages/error_page');
      return cmd.component('tr-error-page');
    },
  },
  {
    path: '/:path*',
    action: async (_ctx, cmd) => {
      await import(/* webpackChunkName: "not_found_page" */ './pages/not_found_page');
      return cmd.component('tr-not-found-page');
    },
  },
]);
