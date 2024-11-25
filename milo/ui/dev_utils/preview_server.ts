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

import * as fs from 'node:fs';
import * as path from 'node:path';

import { PluginOption } from 'vite';

import { getLocalUiVersionJs } from './ui_version_js_utils';

function getContentType(filename: string) {
  if (filename.endsWith('.js.map')) {
    return 'application/json';
  }
  if (filename.endsWith('.js')) {
    return 'text/javascript';
  }
  throw new Error(
    `file ${JSON.stringify(filename)} does not have a known MIME type`,
  );
}

export function previewServer(
  assetDir: string,
  env: Record<string, string | undefined>,
): PluginOption {
  return {
    name: 'luci-ui-preview-server',
    configurePreviewServer: (server) => {
      // Serve `/ui_version.js` during local preview. During local preview, we
      // are serving a local build. The UI's version is different from the
      // active staging UI version.
      server.middlewares.use((req, res, next) => {
        if (req.url !== '/ui_version.js') {
          return next();
        }
        res.setHeader('content-type', 'text/javascript');
        res.end(getLocalUiVersionJs(env));
      });

      // Serve files in `./dist/` but not in `./dist/ui/` (e.g. `root_sw.js`).
      server.middlewares.use((req, res, next) => {
        if (!req.url || req.url.match(/^\/(ui(\/.*)?)?$/)) {
          return next();
        }
        const stat = fs.statSync(path.join(assetDir, req.url), {
          throwIfNoEntry: false,
        });
        if (!stat) {
          return next();
        }

        res.writeHead(200, {
          'content-type': getContentType(req.url),
        });
        const readStream = fs.createReadStream(path.join(assetDir, req.url));
        readStream.pipe(res);
      });
    },
  };
}
