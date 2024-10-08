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

import * as path from 'node:path';

import { Plugin } from 'vite';

/**
 * Vite resolves external resources with relative URLs (URLs without a domain
 * name) inconsistently.
 *
 * Inject `<script src="/ui_version.js"><script>` via a plugin to prevent Vite
 * from conditionally prepending "/ui" prefix onto the URL during local
 * development.
 */
export function stableUiVersionJsLinkPlugin(): Plugin {
  return {
    name: 'luci-ui-stable-ui-version-js',
    transformIndexHtml: (html) => ({
      html,
      tags: [
        {
          tag: 'script',
          attrs: {
            src: '/ui_version.js',
          },
          injectTo: 'head-prepend',
        },
      ],
    }),
  };
}

/**
 * Construct a `ui_version.js` file from environment variables.
 */
export function getLocalUiVersionJs(env: Record<string, string | undefined>) {
  const localUiVersion = env['VITE_LOCAL_UI_VERSION'];
  const localUiVersionJs = `self.UI_VERSION = '${localUiVersion}';\n`;
  return localUiVersionJs;
}

/**
 * Get a virtual-ui-version-js plugin so we can import ui_version.js in the
 * service workers with the correct syntax required by different environments.
 */
export function getVirtualUiVersionJsPlugin(
  mode: string,
  env: Record<string, string | undefined>,
): Plugin {
  return {
    name: 'luci-ui-virtual-ui-version-js',
    resolveId: (id, importer) => {
      if (id !== 'virtual:ui_version.js') {
        return null;
      }

      // `importScripts` is only available in workers.
      // Ensure this module is only used by service workers.
      if (
        !['src/sw/root_sw.ts', 'src/sw/ui_sw.ts']
          .map((p) => path.join(__dirname, '../', p))
          .includes(importer || '')
      ) {
        throw new Error(
          'virtual:ui_version.js should only be imported by a service worker script.',
        );
      }
      return '\0virtual:ui_version.js';
    },
    load: (id) => {
      if (id !== '\0virtual:ui_version.js') {
        return null;
      }

      // In production, the service worker script cannot be a JS module due to
      // limited browser support. So we need to use `importScripts` instead of
      // `import` to load `/ui_version.js`.
      if (mode !== 'development') {
        return "importScripts('/ui_version.js');";
      }

      // During development, the service worker script can only be a JS module,
      // because it runs through the same pipeline as the rest of the scripts.
      // It cannot use the `importScripts`. So we inject the ui_version directly.
      return getLocalUiVersionJs(env);
    },
  };
}
