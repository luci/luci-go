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
export function getLocalUiVersionJs(
  env: Record<string, string | undefined>,
  versionType: 'new-ui' | 'old-ui',
) {
  const localUiVersion = env['VITE_LOCAL_UI_VERSION'];
  const localUiVersionJs = `
    self.UI_VERSION = '${localUiVersion}';
    self.UI_VERSION_TYPE = '${versionType}';
  `;
  return localUiVersionJs;
}
