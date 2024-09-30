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

import replace from '@rollup/plugin-replace';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';
import { VitePWA } from 'vite-plugin-pwa';
import tsconfigPaths from 'vite-tsconfig-paths';

import {
  getLocalDevConfigsJs,
  getVirtualConfigsJsPlugin,
  overrideMiloHostPlugin,
} from './dev_utils/configs_js_utils';
import { localAuthPlugin } from './dev_utils/local_auth_plugin';
import { previewServer } from './dev_utils/preview_server';
import { regexpsForRoutes } from './src/generic_libs/tools/react_router_utils';
import { routes } from './src/routes';

/**
 * Get a boolean from the envDir.
 *
 * Return true/false if the value matches 'true'/'false' (case sensitive).
 * Otherwise, return null.
 */
function getBoolEnv(
  envDir: Record<string, string | undefined>,
  key: string,
): boolean | null {
  const value = envDir[key];
  if (value === 'true') {
    return true;
  }
  if (value === 'false') {
    return false;
  }
  return null;
}

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

  const virtualConfigJs = getVirtualConfigsJsPlugin(mode, env);
  const overrideMiloHost = overrideMiloHostPlugin(env);
  const defineRoutesRegex = replace({
    preventAssignment: true,
    include: ['./src/sw/ui_sw.ts'],
    values: {
      // The build pipeline gets confused when the service worker
      // depends on a lazy-loaded assets (which are used in `routes`).
      // Computes the defined routes at build time to avoid polluting
      // the service worker with unwanted dependencies.
      DEFINED_ROUTES_REGEXP: JSON.stringify(
        regexpsForRoutes([{ path: 'ui', children: routes }]).source,
      ),
    },
  });

  return {
    base: '/ui',
    build: {
      // Vite doesn't like index.html to be placed anywhere other than at the
      // root level of the input/output directory.
      // Vite doesn't allow us to output any asset to a place outside of the
      // defined output directory.
      // We want to place root_sw.js in the parent directory of Vite's output
      // directory because that closely matches how those files are served.
      // e.g. `root_sw.js` is served at `/root_sw.js`, while other assets are
      // served at `/ui/` or `/ui/immutable/`.
      // As a result, we need to output Vite generated assets to `dist/ui` and
      // split the `root_sw.js` generation to a separate built step.
      outDir: 'dist/ui',
      assetsDir: 'immutable',
      sourcemap: true,
      rollupOptions: {
        // Silence source map generattion warning. This is a workaround for
        // https://github.com/vitejs/vite/issues/15012.
        onwarn: (warning, defaultHandler) =>
          warning.code !== 'SOURCEMAP_ERROR' && defaultHandler(warning),
      },
      // Set to 8 MB to silence warnings. The files are cached by service worker
      // anyway. Large chunks won't hurt much.
      chunkSizeWarningLimit: Math.pow(2, 13),
    },
    assetsInclude: ['RELEASE_NOTES.md'],
    plugins: [
      replace({
        preventAssignment: true,
        // We have different building pipeline for dev/test/production builds.
        // Limits the impact of the pipeline-specific features to only the
        // entry files for better consistently across different builds.
        include: ['./src/main.tsx'],
        values: {
          ENABLE_UI_SW: JSON.stringify(
            getBoolEnv(env, 'VITE_ENABLE_UI_SW') ?? true,
          ),
        },
      }),
      overrideMiloHost,
      defineRoutesRegex,
      virtualConfigJs,
      {
        name: 'inject-configs-js-in-html',
        // Vite resolves external resources with relative URLs (URLs without a
        // domain name) inconsistently.
        // Inject `<script src="/configs.js" ><script>` via a plugin to prevent
        // Vite from conditionally prepending "/ui" prefix onto the URL during
        // local development.
        transformIndexHtml: (html) => ({
          html,
          tags: [
            {
              tag: 'script',
              attrs: {
                src: '/configs.js',
              },
              injectTo: 'head-prepend',
            },
          ],
        }),
      },
      {
        // We cannot implement this as a virtual module or a replace variable
        // plugin because we need all the modules to be generated.
        name: 'preload-modules-handle',
        transformIndexHtml: (html, ctx) => {
          const jsChunks = Object.keys(ctx.bundle || {})
            // Prefetch all JS files so users are less likely to run into errors
            // when navigating between views after a new LUCI UI version is
            // deployed.
            .filter((name) => name.match(/^immutable\/.+\.js$/))
            // Don't need to prefetch the entry file since it's loaded as a
            // script tag already.
            .filter((name) => name !== ctx.chunk?.fileName)
            .map((name) => `/ui/${name}`);

          // Sort the chunks to ensure the generated prefetch tags are
          // deterministic.
          jsChunks.sort();

          const preloadScript = `
            function preloadModules() {
              ${jsChunks.map((c) => `import('${c}');\n`).join('')}
            }
          `;

          return {
            html,
            tags: [
              {
                tag: 'script',
                children: preloadScript,
                injectTo: 'head',
              },
            ],
          };
        },
      },
      localAuthPlugin(),
      {
        name: 'dev-server',
        configureServer: (server) => {
          // Serve `/root_sw.js` in local development environment.
          server.middlewares.use((req, _res, next) => {
            if (req.url === '/root_sw.js') {
              req.url = '/ui/src/sw/root_sw.ts';
            }
            return next();
          });

          // Serve `/configs.js` in local development environment.
          // We don't want to define `SETTINGS` directly because that would
          // prevent us from testing the service worker's `GET '/configs.js'`
          // handler.
          server.middlewares.use((req, res, next) => {
            if (req.url !== '/configs.js') {
              return next();
            }
            res.setHeader('content-type', 'text/javascript');
            res.end(getLocalDevConfigsJs(env));
          });
        },
      },
      previewServer(path.join(__dirname, 'dist')),
      react({
        babel: {
          configFile: true,
        },
      }),
      // Support custom path mapping declared in tsconfig.json.
      tsconfigPaths(),
      // Needed to add workbox powered service workers, which enables us to
      // implement some cache strategy that's not possible with regular HTTP
      // cache due to dynamic URLs.
      VitePWA({
        injectRegister: null,
        // We cannot use the simpler 'generateSW' mode because
        // 1. we need to inject custom scripts that import other files, and
        // 2. in 'generateSW' mode, custom scripts can only be injected via
        //    `workbox.importScripts`, which doesn't support ES modules, and
        // 3. in 'generateSW' mode, we need to build the custom script to be
        //    imported by the service worker as an entry file, which means the
        //    resulting bundle may contain ES import statements due to
        //    [vite/rollup not supporting disabling code splitting][1].
        //
        // [1]: https://github.com/rollup/rollup/issues/2756
        strategies: 'injectManifest',
        srcDir: 'src/sw',
        filename: 'ui_sw.ts',
        devOptions: {
          enabled: true,
          // During development, the service worker script can only be a JS
          // module, because it runs through the same pipeline as the rest of
          // the scripts, which produces ES modules.
          type: 'module',
          navigateFallback: 'index.html',
        },
        manifest: {
          theme_color: '#1976d2',
        },
        injectManifest: {
          globPatterns: ['**/*.{js,css,html,svg,png}'],
          buildPlugins: {
            vite: [
              defineRoutesRegex,
              virtualConfigJs,
              overrideMiloHost,
              tsconfigPaths(),
            ],
          },
          // Set to 8 MB. Some files might be larger than the default.
          maximumFileSizeToCacheInBytes: Math.pow(2, 23),
        },
      }),
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
