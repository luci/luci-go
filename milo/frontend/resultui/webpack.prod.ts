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

import webpack from 'webpack';
import merge from 'webpack-merge';

import common from './webpack.common';

// TODO(weiweilin): once ts-loader is able to produce hash independent of
// the project's absolute path
//  * remove HashOutputPlugin
//  * replace [chunkhash] with [contenthash]
//  * enable source-map
//  * simplify prod config because it should share more commonalities with
//    ./webpack.common
// Related: https://github.com/TypeStrong/ts-loader/pull/1085

// Use require since webpack-plugin-hash-output has no type declaration files.
// tslint:disable-next-line: variable-name
const HashOutputPlugin = require('webpack-plugin-hash-output');

const config: webpack.Configuration = merge.strategy({plugins: 'prepend'})(common, {
  output: {
    filename: '[name].[chunkhash].bundle.js',
    chunkFilename: '[name].[chunkhash].bundle.js',
  },
  mode: 'production',
  plugins: [
    new HashOutputPlugin(),
  ],
});

// Default export is required by webpack.
// tslint:disable-next-line: no-default-export
export default config;
