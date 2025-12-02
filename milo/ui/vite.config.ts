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

import * as path from 'node:path';

import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';
import tsconfigPaths from 'vite-tsconfig-paths';

import { devServer } from './dev_utils/dev_server';
import { localAuthPlugin } from './dev_utils/local_auth_plugin';
import { previewServer } from './dev_utils/preview_server';
import {
  overrideMiloHostPlugin,
  stableSettingsJsLinkPlugin,
} from './dev_utils/settings_js_utils';
import { stableUiVersionJsLinkPlugin } from './dev_utils/ui_version_js_utils';

export default defineConfig(({ mode }) => {
  const env = {
    ...Object.fromEntries(
      Object.entries(process.env).filter(([k]) => k.startsWith('VITE_')),
    ),
    ...loadEnv(
      mode,
      process.cwd(),
      // Our production UI build should not depend on any env vars.
      // Still include `VITE_LOCAL_` env vars. They are used when previewing a
      // production build locally.
      mode === 'production' ? 'VITE_LOCAL_' : 'VITE_',
    ),
  };

  // Allow customizing the output directory so we can target another build when
  // running integration tests in a builder.
  const baseOutDir = env['VITE_LOCAL_BASE_OUT_DIR'] ?? 'dist';

  const overrideMiloHost = overrideMiloHostPlugin(env);

  return {
    base: '/ui',
    clearScreen: false,
    define: {
      // Some Babel packages check process.env variables which don't exist
      // in the browser, causing `ReferenceError: process is not defined`.
      // Defining them here as 'false' ensures these checks work without errors.
      'process.env.BABEL_8_BREAKING': 'false',
      'process.env.BABEL_TYPES_8_BREAKING': 'false',
    },
    build: {
      outDir: path.join(baseOutDir, '/ui'),
      assetsDir: 'immutable',
      sourcemap: true,
      rollupOptions: {
        // Silence source map generation warning. This is a workaround for
        // https://github.com/vitejs/vite/issues/15012.
        onwarn: (warning, defaultHandler) =>
          warning.code !== 'SOURCEMAP_ERROR' && defaultHandler(warning),
      },
    },
    assetsInclude: ['RELEASE_NOTES.md'],
    plugins: [
      stableUiVersionJsLinkPlugin(),
      stableSettingsJsLinkPlugin(),
      overrideMiloHost,
      localAuthPlugin({
        // Only enable if explicitly set to 'true'. undefined or 'false' will result in false.
        enableAndroidScopes: env['VITE_LOCAL_ENABLE_ANDROID_SCOPES'] === 'true',
      }),
      devServer(env),
      previewServer(path.join(__dirname, baseOutDir), env),
      react({
        babel: {
          configFile: true,
        },
      }),
      // Support custom path mapping declared in tsconfig.json.
      tsconfigPaths(),
    ],
    server: {
      port: 8080,
      strictPort: true,
      // Proxy the queries to `self.location.host` to the configured milo server
      // (typically https://luci-milo-dev.appspot.com) since we don't run the
      // milo go server on the same host.
      proxy: {
        '^(?!/ui/.*$)': {
          target: env['VITE_LOCAL_PROXY_URL'],
          changeOrigin: true,
          secure: false,
        },
      },
    },
    preview: {
      port: 8000,
    },
  };
});
