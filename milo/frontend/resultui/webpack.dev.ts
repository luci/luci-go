// Copyright 2020 The LUCI Authors.
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

import 'cypress';
import 'webpack-dev-server';
import fs from 'fs';
import { createProxyMiddleware } from 'http-proxy-middleware';
import path from 'path';
import { DefinePlugin } from 'webpack';
import merge from 'webpack-merge';
import Workbox from 'workbox-webpack-plugin';

import common from './webpack.common';
const cypressConfig: Cypress.ConfigOptions = require('./cypress.json');

const DEBUG_SW = process.env.DEBUG_SW === 'true';

export default merge(common, {
  mode: 'development',
  devtool: 'eval-source-map',
  // Service workers makes live/hot reload harder, so it should be disabled by
  // default in dev mode.
  plugins: [
    new DefinePlugin({
      ENABLE_UI_SW: JSON.stringify(DEBUG_SW),
    }),
    ...(DEBUG_SW
      ? [
          new Workbox.GenerateSW({
            // Without this, new release will not take effect until users close
            // all build page tabs.
            skipWaiting: true,
            navigateFallback: '/ui/index.html',
            sourcemap: true,
            importScriptsViaChunks: ['service-worker-ext'],
            // Set to 5 MB to suppress the warning in dev mode.
            // Dev bundles are naturally larger as they have inline source maps.
            maximumFileSizeToCacheInBytes: 5242880,
          }),
        ]
      : []),
  ],

  devServer: {
    hot: false,
    https: true,
    // Live reload causes the page to enter refresh loop when debugging SW.
    liveReload: !DEBUG_SW,
    client: {
      overlay: false,
    },
    static: {
      directory: path.join(__dirname, './out/'),
    },
    historyApiFallback: {
      index: '/ui/index.html',
    },
    port: Number(new URL(cypressConfig.baseUrl || 'https://localhost:8080').port),
    setupMiddlewares: (middlewares, devServer) => {
      // This won't happen, but it helps TSC infer the type.
      if (!devServer.app) {
        throw new Error('app not initialized');
      }

      devServer.app.use((req, _res, next) => {
        // Host root-sw.js at root so it can have root scope.
        if (/^\/(root-sw\.js(\.map)?)$/.test(req.path)) {
          req.url = req.url.slice(0, req.url.length - req.path.length) + '/ui' + req.path;
        }
        next();
      });

      devServer.app.get('/configs.js', async (_req, res) => {
        res.set('content-type', 'application/javascript');
        const appConfigs = JSON.parse(fs.readFileSync(path.join(__dirname, 'dev-configs/configs.json'), 'utf8'));
        const configsTemplate = fs.readFileSync(path.join(__dirname, 'assets/configs.template.js'), 'utf8');
        const config = configsTemplate
          .replace('{{.ResultDB.Host}}', appConfigs.RESULT_DB.HOST)
          .replace('{{.Buildbucket.Host}}', appConfigs.BUILDBUCKET.HOST)
          .replace('{{.Weetbix.Host}}', appConfigs.WEETBIX.HOST);
        res.send(config);
      });

      devServer.app.use(
        /^(?!\/ui\/).*/,
        createProxyMiddleware({
          // This attribute is required. However the value will be overridden by
          // the router option. So the value doesn't matter.
          target: 'https://luci-milo-dev.appspot.com',
          router: () => {
            const localDevConfigs = JSON.parse(
              fs.readFileSync(path.join(__dirname, 'dev-configs/local-dev-configs.json'), 'utf8')
            );
            return localDevConfigs.milo.url;
          },
          changeOrigin: true,
          secure: false,
        })
      );

      return middlewares;
    },
  },
});
