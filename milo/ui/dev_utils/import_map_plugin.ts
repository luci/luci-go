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

import { createHash } from 'crypto';
import * as path from 'path';

import { OutputAsset, OutputChunk } from 'rollup';
import { PluginOption } from 'vite';

// Note that 7d608c4 is not the actual hash, it's just a randomly generated
// string that reduces the chance of collision.
export const HASH_TAG = '<luci-ui-contenthash-7d608c4>';

function isOutputChunkWithHashTag(
  chunk: OutputAsset | OutputChunk,
): chunk is OutputChunk {
  return chunk.type === 'chunk' && chunk.fileName.includes(HASH_TAG);
}

/**
 * A Vite plugin that uses the import map to support adding hashes to the
 * generated files without adding hashes to the import statements. As a result,
 * updating a single file won't trigger cascading hash updates and therefore
 * cascading cache invalidation.
 *
 * TODO: As of 2025-01-09, there's at least one case where updating a small
 * number of files will trigger mass cache invalidation. Certain changes can
 * change symbol assignment after minification in most (all?) files causing a
 * wide spread cache invalidation. The exact trigger is unknown.
 *
 * Use `HASH_TAG` instead of [hash] in the entry and chunk filenames to
 * ensure that the hash is not resolved by the bundler at build time (and
 * therefore prevent cascading hash updates).
 */
export function importMapPlugin(): PluginOption {
  let base = '';
  let importMap: { [key: string]: string } = {};
  return {
    name: 'luci-ui-import-map',
    configResolved(config) {
      base = config.base;
      importMap = {};
    },
    generateBundle(_opts, bundle) {
      // We only care about JS chunks that use our HASH_TAG placeholder
      for (const chunk of Object.values(bundle).filter(
        isOutputChunkWithHashTag,
      )) {
        // Calculate Hashes and New Filenames
        const contentHash = createHash('md5')
          .update(chunk.code)
          .digest('hex')
          .slice(0, 8);

        const originalJsName = chunk.fileName;
        const newJsName = originalJsName.replace(HASH_TAG, contentHash);
        const originalMapName = `${originalJsName}.map`;
        const newMapName = `${newJsName}.map`;

        chunk.fileName = newJsName;

        delete bundle[originalJsName];
        bundle[newJsName] = chunk;

        // Manually update the sourceMappingURL comment in the JS code
        chunk.code = chunk.code.replace(
          `//# sourceMappingURL=${path.basename(originalMapName)}`,
          `//# sourceMappingURL=${path.basename(newMapName)}`,
        );

        // Find the Corresponding Sourcemap Asset and update
        // correspondingly if exists
        const sourcemapAsset = bundle[originalMapName];
        if (!sourcemapAsset) {
          // If no sourcemap, we can't proceed with this chunk
          continue;
        }

        sourcemapAsset.fileName = newMapName;

        delete bundle[originalMapName];
        bundle[newMapName] = sourcemapAsset;

        // Update the import map for index.html
        importMap[path.join(base, originalJsName)] = path.join(base, newJsName);
      }
    },
    transformIndexHtml: (html) => {
      return {
        html,
        tags: [
          {
            tag: 'script',
            attrs: {
              type: 'importmap',
            },
            children: `\n${JSON.stringify({ imports: importMap }, undefined, 2)}\n`,
            injectTo: 'head-prepend',
          },
        ],
      };
    },
  };
}
