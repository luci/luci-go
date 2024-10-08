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

import { PluginOption } from 'vite';

import { getLocalDevSettingsJs } from './settings_js_utils';
import { getLocalUiVersionJs } from './ui_version_js_utils';

export function devServer(
  env: Record<string, string | undefined>,
): PluginOption {
  return {
    name: 'luci-ui-dev-server',
    configureServer: (server) => {
      // Serve `/root_sw.js` in local development environment.
      server.middlewares.use((req, _res, next) => {
        if (req.url === '/root_sw.js') {
          req.url = '/ui/src/sw/root_sw.ts';
        }
        return next();
      });

      // Serve `/ui_version.js` in local development environment.
      // We don't want to define `UI_VERSION` directly because that would
      // prevent us from testing the service worker's `GET '/ui_version.js'`
      // handler.
      server.middlewares.use((req, res, next) => {
        if (req.url !== '/ui_version.js') {
          return next();
        }
        res.setHeader('content-type', 'text/javascript');
        res.end(getLocalUiVersionJs(env));
      });

      // Serve `/settings.js` in local development environment.
      // We don't want to define `SETTINGS` directly because that would
      // prevent us from testing the service worker's `GET '/settings.js'`
      // handler.
      server.middlewares.use((req, res, next) => {
        if (req.url !== '/settings.js') {
          return next();
        }
        res.setHeader('content-type', 'text/javascript');
        res.end(getLocalDevSettingsJs(env));
      });
    },
  };
}
