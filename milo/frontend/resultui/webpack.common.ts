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
import webpack, { DefinePlugin } from 'webpack';

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
      {
        test: /\.css$/,
        loader: 'lit-css-loader',
      },
    ],
  },
  resolve: {
    extensions: ['.js', '.ts', '.css'],
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
        defaultVendors: {
          test: /[\\/]node_modules[\\/]/,
          name(module: {context: string}) {
            const packageName = module.context.match(/[\\/]node_modules[\\/](.*?)([\\/]|$)/)![1];
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
    new DefinePlugin({ENABLE_GA: JSON.stringify(process.env.ENABLE_GA === 'true')}),
  ],
};

// Default export is required by webpack.
// tslint:disable-next-line: no-default-export
export default config;
