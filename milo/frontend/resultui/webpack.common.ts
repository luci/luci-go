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

import path from 'path';

import { CleanWebpackPlugin } from 'clean-webpack-plugin';
import HtmlWebpackHarddiskPlugin from 'html-webpack-harddisk-plugin';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import { createProxyMiddleware } from 'http-proxy-middleware';
import webpack from 'webpack';

import { readFileSync } from 'fs';

const config: webpack.Configuration = {
  entry: {
    index: './src/index.ts',
  },
  output: {
    path: path.resolve(__dirname, './out/static/dist/scripts/'),
    publicPath: '/static/dist/scripts/',
    filename: '[name].[contenthash].bundle.js',
    chunkFilename: '[name].[contenthash].bundle.js',
  },
  module: {
    rules: [
      {
        test: /\.ts$/,
        use: {
          loader: 'ts-loader',
          options: {
            configFile: 'tsconfig.build.json',
          },
        },
        exclude: /node_modules/,
      },
    ],
  },
  resolve: {
    extensions: ['.js', '.ts'],
  },
  externals: {
    'configs': 'CONFIGS',
  },
  optimization: {
    runtimeChunk: 'single',
    splitChunks: {
      chunks: 'all',
      maxInitialRequests: Infinity,
      minSize: 0,
      cacheGroups: {
        vendor: {
          test: /[\\/]node_modules[\\/]/,
          name(module) {
            const packageName = module.context.match(/[\\/]node_modules[\\/](.*?)([\\/]|$)/)[1];
            return `npm.${packageName}`;
          },
        },
      },
    },
  },
  plugins: [
    new CleanWebpackPlugin(),
    new HtmlWebpackPlugin({
      alwaysWriteToDisk: true,
      template: path.resolve(__dirname, './index.html'),
      filename: path.resolve(__dirname, './out/index.html'),
    }),
    new HtmlWebpackHarddiskPlugin(),
  ],
  devServer: {
    contentBase: path.join(__dirname, './out/'),
    historyApiFallback: true,
    before: (app) => {
      const devConfig = require('./local-dev-config.json');
      app.get('/configs.js', async (_req, res) => {
        res.set('context-type', 'application/javascript');
        const configsTemplate = readFileSync('./configs.template.js', 'utf8');
        const config = configsTemplate
          .replace('{{.ResultDB.Host}}', devConfig.result_db.host)
          .replace('{{.OAuth2.ClientID}}', devConfig.client_id);
        res.send(config);
      });
      app.use(/^(?!\/(ui|static\/(dist|style))\/).*/, createProxyMiddleware({target: devConfig.milo.url, changeOrigin: true}));
    },
  },
};

// Default export is required by webpack.
// tslint:disable-next-line: no-default-export
export default config;
