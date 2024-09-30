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
 * Insert a `preloadModules` global function.
 *
 * `preloadModules` preloads all lazy-loaded JS assets when invoked.
 */
export function preloadModulesPlugin(): Plugin {
  return {
    // We cannot implement this as a virtual module or a replace variable
    // plugin because we need all the modules to be generated.
    name: 'luci-ui-preload-modules',
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
  };
}
