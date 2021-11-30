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

import fs from 'fs';
import { Config } from 'karma';
import path from 'path';
import { DefinePlugin, ProvidePlugin } from 'webpack';

import webpackConfig from './webpack.common';

module.exports = (config: Config) => {
  const isDebug = process.argv.some((arg) => arg === '--debug');
  config.set({
    protocol: 'https:',
    httpsServerOptions: {
      key: fs.readFileSync(path.join(__dirname, 'dev-configs/cert.key')),
      cert: fs.readFileSync(path.join(__dirname, 'dev-configs/cert.pem')),
    },

    // base path that will be used to resolve all patterns (eg. files, exclude)
    basePath: '',

    // frameworks to use
    // available frameworks: https://npmjs.org/browse/keyword/karma-adapter
    frameworks: ['webpack', 'mocha', 'chai'],

    // list of files / patterns to load in the browser
    files: [{ pattern: 'index_test.ts', watched: false }],

    // list of files / patterns to exclude
    exclude: [],

    // preprocess matching files before serving them to the browser
    // available preprocessors:
    // https://npmjs.org/browse/keyword/karma-preprocessor
    preprocessors: {
      'index_test.ts': ['webpack', 'sourcemap'],
    },

    plugins: [
      'karma-chrome-launcher',
      'karma-webpack',
      'karma-sourcemap-loader',
      'karma-mocha',
      'karma-mocha-reporter',
      'karma-chai',
    ],

    webpack: {
      // webpack configuration
      devtool: 'inline-source-map',
      mode: 'development',
      module: webpackConfig.module,
      resolve: webpackConfig.resolve,
      externals: webpackConfig.externals,
      output: {
        // Use relative file path for cleaner stack trace with navigable source
        // location in the terminal.
        devtoolModuleFilenameTemplate: '[resource-path]',
      },
      optimization: {
        // Disable splitChunks. Otherwise sourcemap won't work in the terminal.
        // https://github.com/ryanclark/karma-webpack/issues/493
        splitChunks: false,
      },
      plugins: [
        new DefinePlugin({
          CONFIGS: fs.readFileSync('./dev-configs/configs.json', 'utf-8'),
          WORKBOX_PROMISE: 'Promise.race([])',
          // JS values need to be converted to JSON notation.
          ENABLE_GA: JSON.stringify(false),
          VISIT_ID: JSON.stringify('0'),
          CACHED_AUTH_STATE: JSON.stringify(null),
          TIME_ORIGIN: JSON.stringify(0),
        }),
        new ProvidePlugin({
          process: 'process/browser.js',
        }),
      ],
    },

    webpackMiddleware: {
      stats: 'errors-only',
    },

    // test results reporter to use
    // possible values: 'dots', 'progress'
    // available reporters: https://npmjs.org/browse/keyword/karma-reporter
    reporters: ['mocha'],

    // web server port
    port: 9876,

    // enable / disable colors in the output (reporters and logs)
    colors: true,

    // level of logging
    // possible values: config.LOG_DISABLE || config.LOG_ERROR ||
    // config.LOG_WARN || config.LOG_INFO || config.LOG_DEBUG
    logLevel: config.LOG_INFO,

    // enable / disable watching file and executing tests whenever any file
    // changes
    autoWatch: true,

    // start these browsers
    // available browser launchers:
    // https://npmjs.org/browse/keyword/karma-launcher
    browsers: isDebug ? ['Chrome'] : ['ChromeHeadless'],

    // Continuous Integration mode
    // if true, Karma captures browsers, runs the tests and exits
    singleRun: false,

    // Concurrency level
    // how many browser should be started simultaneous
    concurrency: Infinity,
  });
};
