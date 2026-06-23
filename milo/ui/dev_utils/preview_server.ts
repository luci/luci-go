// Copyright 2026 The LUCI Authors.
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

import { PluginOption, loadEnv } from 'vite';

import { getLocalDevSettingsJs } from './settings_js_utils';
import { getLocalUiVersionJs } from './ui_version_js_utils';

export function previewServer(
  _assetDir: string,
  env: Record<string, string | undefined>,
): PluginOption {
  return {
    name: 'luci-ui-preview-server',
    configurePreviewServer: (server) => {
      // Load development environment variables to populate settings.js
      // during preview (e.g. in E2E tests).
      const devEnv = loadEnv(
        'development',
        server.config.envDir,
        server.config.envPrefix,
      );
      const settingsEnv = { ...devEnv, ...env };

      // Serve `/ui_version.js` during local preview. During local preview, we
      // are serving a local build. The UI's version is different from the
      // active staging UI version.
      server.middlewares.use((req, res, next) => {
        if (req.url !== '/ui_version.js') {
          return next();
        }
        res.setHeader('content-type', 'text/javascript');
        res.end(
          getLocalUiVersionJs(
            env,
            req.headers.cookie?.includes('USER_INITIATED_ROLLBACK=true')
              ? 'old-ui'
              : 'new-ui',
          ),
        );
      });

      // Serve `/settings.js` during local preview.
      server.middlewares.use((req, res, next) => {
        if (req.url !== '/settings.js') {
          return next();
        }
        res.setHeader('content-type', 'text/javascript');
        try {
          res.end(getLocalDevSettingsJs(settingsEnv));
        } catch (e) {
          res.statusCode = 500;
          res.end(`Failed to generate settings.js: ${String(e)}`);
        }
      });
    },
  };
}
