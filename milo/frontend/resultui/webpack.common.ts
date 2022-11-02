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

import { CleanWebpackPlugin } from 'clean-webpack-plugin';
import CopyWebpackPlugin from 'copy-webpack-plugin';
import HtmlWebpackHarddiskPlugin from 'html-webpack-harddisk-plugin';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import path from 'path';
import { Configuration, DefinePlugin } from 'webpack';

const config: Configuration = {
  entry: {
    index: './src/index.ts',
    'service-worker-ext': './src/service_workers/service_worker_ext.ts',
    'root-sw': './src/service_workers/root_sw.ts',
  },
  output: {
    path: path.resolve(__dirname, './out/'),
    publicPath: '/ui/',
    filename: ({ chunk }) => (chunk?.name === 'root-sw' ? '[name].js' : 'immutable/[name].[contenthash].bundle.js'),
    chunkFilename: 'immutable/[name].[contenthash].bundle.js',
    trustedTypes: {
      policyName: 'milo#webpack',
    },
  },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: {
          loader: 'ts-loader',
          options: {
            configFile: 'tsconfig.build.json',
          },
        },
        exclude: /node_modules/,
      },
      {
        test: /\.css$/,
        loader: 'lit-css-loader',
      },
    ],
  },
  resolve: {
    extensions: ['.js', '.jsx', '.ts', '.tsx', '.css'],
  },
  externals: {
    configs: 'CONFIGS',
  },
  optimization: {
    // Disable runtime chunk so standalone scripts can be executed/imported
    // independently.
    runtimeChunk: false,
    splitChunks: {
      chunks: (chunk) => !['service-worker-ext', 'root-sw'].includes(chunk.name),
      maxInitialRequests: Infinity,
      minSize: 0,
      cacheGroups: {
        defaultVendors: {
          test: /[\\/]node_modules[\\/]/,
          name(module: { context: string }) {
            const packageName = module.context.match(/[\\/]node_modules[\\/](.*?)([\\/]|$)/)![1];
            return `npm.${encodeURIComponent(packageName)}`;
          },
        },
      },
    },
  },
  performance: {
    // Set to 1 MB.
    // This is fine as files should be cached by the service worker anyway.
    maxEntrypointSize: 1048576,
    maxAssetSize: 1048576,
  },
  plugins: [
    new CleanWebpackPlugin(),
    new CopyWebpackPlugin({
      patterns: [{ from: 'assets', filter: (p) => !/\.template\./.test(p) }],
    }),
    new HtmlWebpackPlugin({
      alwaysWriteToDisk: true,
      template: path.resolve(__dirname, './assets/index.template.ejs'),
      filename: path.resolve(__dirname, './out/index.html'),
      // We should only include one entry chunk so modules won't be initialized
      // multiple times.
      chunks: ['index'],
    }),
    new HtmlWebpackHarddiskPlugin(),
    new DefinePlugin({ ENABLE_GA: JSON.stringify(process.env.ENABLE_GA === 'true') }),
  ],
};

export default config;
