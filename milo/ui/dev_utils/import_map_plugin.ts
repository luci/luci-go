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

import { PluginOption } from 'vite';
// While it's possible that the web may have newer/different standards for
// sourcemap URLs. In the worst case, we will only break the source map not
// the actual production code.
const SOURCE_MAPPING_URL_RE = /^\/\/# sourceMappingURL=(.+)$/m;
// Note that 7d608c4 is not the actual hash, it's just a randomly generated
// string that reduces the chance of collision.
export const HASH_TAG = '<luci-ui-contenthash-7d608c4>';
/**
 * Given a URL `path` from a URL `base` path, returns the absolute URL path.
 */
function absUrlPath(base: string, path: string) {
  const PLACEHOLDER_DOMAIN = 'http://placeholder.domain/';
  const url = new URL(path, PLACEHOLDER_DOMAIN + base);
  return url.toString().slice(PLACEHOLDER_DOMAIN.length);
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
      // Map old sourcemap filenames (with content hash placeholder) to new
      // sourcemap filenames (with the real content hash).
      const smFilenameMap: { [key: string]: string } = {};
      for (const chunk of Object.values(bundle)) {
        if (chunk.type !== 'chunk') {
          continue;
        }
        // Replace the HASH_TAG in the filename with actual content hash of the
        // file so we can cache those files without running into version
        // resolution issue.
        const contentHash = createHash('md5')
          .update(chunk.code)
          .digest('hex')
          .slice(0, 8);
        const hashedFilename = chunk.fileName.replace(HASH_TAG, contentHash);
        importMap[path.join(base, chunk.fileName)] = path.join(
          base,
          hashedFilename,
        );
        // Record the sourcemap URL and fill the content hash placeholder. Store
        // the mapping so it can be used to update sourcemap chunk filenames
        // later.
        const match = SOURCE_MAPPING_URL_RE.exec(chunk.code);
        if (match) {
          const resolvedSM = absUrlPath(chunk.fileName, match[1]);
          smFilenameMap[resolvedSM] = absUrlPath(
            chunk.fileName,
            match[1].replace(HASH_TAG, contentHash),
          );
        }
        // Populate the content hash placeholder in the filename and the
        // sourcemap URL.
        chunk.fileName = hashedFilename;
        chunk.code = chunk.code.replace(SOURCE_MAPPING_URL_RE, (url) =>
          url.replace(HASH_TAG, contentHash),
        );
      }
      // Fill the content hash placeholder in all the sourcemap filenames.
      for (const chunk of Object.values(bundle)) {
        if (chunk.type !== 'asset' || !chunk.fileName.endsWith('.map')) {
          continue;
        }
        const newFilename = smFilenameMap[absUrlPath('', chunk.fileName)];
        // It's possible that another plugin can create a sourcemap for a file
        // that's not process by the regular pipeline (e.g. virtual modules?).
        // In that case we don't need to do anything for it if it's not using
        // the HASH_TAG anyway.
        if (!newFilename) {
          continue;
        }
        chunk.fileName = newFilename;
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
