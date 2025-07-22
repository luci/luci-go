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

/**
 * @fileoverview
 *
 * This is the rollup config to build assets that needs to be served at `/`
 * (instead of `/ui/`).
 *
 * Vite doesn't like index.html to be placed anywhere other than at the
 * root level of the input/output directory.
 * Vite doesn't allow us to output any asset to a place outside of the
 * defined output directory.
 * We want to place some assets (e.g. `root_sw.js`) in the parent directory of
 * Vite's output directory because that closely matches how those files are
 * served.
 *
 * As a result, we need to output Vite generated assets to `dist/ui` and use a
 * a separate built step for other assets placed under `dist/`.
 */

import typescript from '@rollup/plugin-typescript';

/** @type {import('rollup').RollupOptions[]} */
const options = [
  {
    input: 'src/sw/root_sw.ts',
    output: {
      file: 'dist/root_sw.js',
      // In production, the service worker script cannot be an ES module due to
      // limited browser support [1].
      // [1]: https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorker#browser_compatibility
      format: 'commonjs',
      sourcemap: true,
      sourcemapPathTransform: (relativePath, _) => {
        const normalized = relativePath.replace(/\\/g, '/');

        // remove leading ../ in the path. In that case they will be considered
        // relative to the root of the host which is ok
        return '/' + normalized.replace(/^(\.\.\/)+/, '');
        // return normalized.replace(/^(\.\.\/)/, '');
      },
    },
    plugins: [typescript()],
  },
];

export default options;
