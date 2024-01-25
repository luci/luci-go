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

import * as fs from 'fs';
import * as path from 'path';

import replace from '@rollup/plugin-replace';
import react from '@vitejs/plugin-react';
import { Plugin, defineConfig, loadEnv } from 'vite';
import { VitePWA as vitePWA } from 'vite-plugin-pwa';
import tsconfigPaths from 'vite-tsconfig-paths';

import { routes } from './src/app/routes';
import { AuthState, msToExpire } from './src/common/api/auth_state';
import { regexpsForRoutes } from './src/generic_libs/tools/react_router_utils';

/**
 * Get a boolean from the envDir.
 *
 * Return true/false if the value matches 'true'/'false' (case sensitive).
 * Otherwise, return null.
 */
function getBoolEnv(
  envDir: Record<string, string>,
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

/**
 * Construct a `configs.js` file from environment variables.
 */
function getLocalDevConfigsJs(env: Record<string, string>) {
  const localVersion = env['VITE_MILO_VERSION'];
  const localSettings: typeof SETTINGS = {
    buildbucket: {
      host: env['VITE_BUILDBUCKET_HOST'],
    },
    swarming: {
      defaultHost: env['VITE_SWARMING_DEFAULT_HOST'],
    },
    resultdb: {
      host: env['VITE_RESULT_DB_HOST'],
    },
    luciAnalysis: {
      host: env['VITE_LUCI_ANALYSIS_HOST'],
    },
    luciBisection: {
      host: env['VITE_LUCI_BISECTION_HOST'],
    },
    sheriffOMatic: {
      host: env['VITE_SHERIFF_O_MATIC_HOST'],
    },
    treeStatus: {
      host: env['VITE_TREE_STATUS_HOST'],
    },
  };

  const localDevConfigsJs =
    `self.VERSION = '${localVersion}';\n` +
    `self.SETTINGS = Object.freeze(${JSON.stringify(
      localSettings,
      undefined,
      2,
    )});\n`;

  return localDevConfigsJs;
}

/**
 * Get a virtual-configs-js plugin so we can import configs.js in the service
 * workers with the correct syntax required by different environments.
 */
function getVirtualConfigsJsPlugin(
  mode: string,
  configsContent: string,
): Plugin {
  return {
    name: 'virtual-configs-js',
    resolveId: (id, importer) => {
      if (id !== 'virtual:configs.js') {
        return null;
      }

      // `importScripts` is only available in workers.
      // Ensure this module is only used by service workers.
      if (
        !['src/app/root_sw.ts', 'src/app/ui_sw.ts']
          .map((p) => path.join(__dirname, p))
          .includes(importer || '')
      ) {
        throw new Error(
          'virtual:configs.js should only be imported by a service worker script.',
        );
      }
      return '\0virtual:config.js';
    },
    load: (id) => {
      if (id !== '\0virtual:config.js') {
        return null;
      }
      // During development, the service worker script can only be a JS module,
      // because it runs through the same pipeline as the rest of the scripts.
      // It cannot use the `importScripts`. So we inject the configs directly.
      // In production, the service worker script cannot be a JS module. Because
      // that has limited browser support. So we need to use `importScripts`
      // instead of `import` to load `/configs.js`.
      return mode === 'development'
        ? configsContent
        : "importScripts('/configs.js');";
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd());

  const localDevConfigsJs = getLocalDevConfigsJs(env);
  const virtualConfigJs = getVirtualConfigsJsPlugin(mode, localDevConfigsJs);

  return {
    base: '/ui',
    build: {
      outDir: 'out',
      assetsDir: 'immutable',
      sourcemap: true,
      rollupOptions: {
        input: {
          index: 'index.html',
          root_sw: './src/app/root_sw.ts',
        },
        output: {
          // 'root_sw' needs to be referenced by URL therefore cannot have a
          // hash in its filename.
          entryFileNames: (chunkInfo) =>
            chunkInfo.name === 'root_sw'
              ? '[name].js'
              : 'immutable/[name]-[hash:8].js',
        },
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
        include: ['./src/app/main.tsx'],
        values: {
          ENABLE_UI_SW: JSON.stringify(
            getBoolEnv(env, 'VITE_ENABLE_UI_SW') ?? true,
          ),
        },
      }),
      virtualConfigJs,
      {
        name: 'inject-configs-js-in-html',
        // Vite resolves external resources with relative URLs (URLs without a
        // domain name) inconsistently.
        // Inject `<script src="/configs.js" ><script>` via a plugin to prevent
        // Vite from conditionally prepending "/ui" prefix onto the URL.
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
        name: 'inject-prefetches-in-html',
        transformIndexHtml: (html, ctx) => {
          const jsChunks = Object.keys(ctx.bundle || {})
            // Prefetch all JS files so users are less likely to run into errors
            // when navigating between views after a new LUCI UI version is
            // deployed.
            .filter((name) => name.match(/^immutable\/.+\.js$/))
            // Don't need to prefetch the entry file since it's loaded as a
            // script tag already.
            .filter((name) => name !== ctx.chunk?.fileName);

          // Sort the chunks to ensure the generated prefetch tags are
          // deterministic.
          jsChunks.sort();

          return {
            html,
            tags: jsChunks.map((chunkName) => ({
              tag: 'link',
              attrs: {
                rel: 'prefetch',
                href: `/ui/${chunkName}`,
                as: 'script',
              },
              injectTo: 'head',
            })),
          };
        },
      },
      {
        name: 'dev-server',
        configureServer: (server) => {
          // Serve `/root_sw.js`
          server.middlewares.use((req, _res, next) => {
            if (req.url === '/root_sw.js') {
              req.url = '/ui/src/app/root_sw.ts';
            }
            return next();
          });

          // Serve `/configs.js` during development.
          // We don't want to define `SETTINGS` directly because that would
          // prevent us from testing the service worker's `GET '/configs.js'`
          // handler.
          server.middlewares.use((req, res, next) => {
            if (req.url !== '/configs.js') {
              return next();
            }
            res.setHeader('content-type', 'application/javascript');
            res.end(localDevConfigsJs);
          });

          // Return a predefined auth state object if a valid
          // `auth_state.local.json` exists.
          server.middlewares.use((req, res, next) => {
            if (req.url !== '/auth/openid/state') {
              return next();
            }

            let authState: AuthState;
            try {
              authState = JSON.parse(
                fs.readFileSync('auth_state.local.json', 'utf8'),
              );
              if (msToExpire(authState) < 10000) {
                return next();
              }
            } catch (_e) {
              return next();
            }

            res.setHeader('content-type', 'application/json');
            res.end(JSON.stringify(authState));
          });

          // When VitePWA is enabled in -dev mode, the entry files are somehow
          // resolved to the wrong paths. We need to remap them to the correct
          // paths.
          server.middlewares.use((req, _res, next) => {
            if (req.url === '/ui/src/index.tsx') {
              req.url = '/ui/src/main.tsx';
            }
            if (req.url === '/ui/src/styles/style.css') {
              req.url = '/ui/src/common/styles/style.css';
            }
            return next();
          });
        },
      },
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
      vitePWA({
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
        srcDir: 'src/app',
        filename: 'ui_sw.ts',
        outDir: 'out',
        devOptions: {
          enabled: true,
          // During development, the service worker script can only be a JS
          // module, because it runs through the same pipeline as the rest of
          // the scripts, which produces ES modules.
          type: 'module',
          navigateFallback: 'index.html',
        },
        injectManifest: {
          globPatterns: ['**/*.{js,css,html,svg,png}'],
          plugins: [
            replace({
              preventAssignment: true,
              include: ['./src/app/ui_sw.ts'],
              values: {
                // The build pipeline gets confused when the service worker
                // depends on a lazy-loaded assets (which are used in `routes`).
                // Computes the defined routes at build time to avoid polluting
                // the service worker with unwanted dependencies.
                DEFINED_ROUTES_REGEXP: JSON.stringify(
                  regexpsForRoutes([{ path: 'ui', children: routes }]).source,
                ),
              },
            }),
            virtualConfigJs,
            tsconfigPaths(),
          ],
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
        '^(?!/ui(/.*)?$)': {
          target: env['VITE_MILO_URL'],
          changeOrigin: true,
          secure: false,
        },
      },
    },
  };
});
