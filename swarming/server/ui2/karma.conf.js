// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

process.env.CHROME_BIN = require("puppeteer").executablePath();

let webpackConfig = require("./webpack.config.js");
// Webpack 3+ configs can be either objects or functions that produce the
// config object. Karma currently doesn't handle the latter, so do it
// ourselves here.
if (typeof webpackConfig === "function") {
  webpackConfig = webpackConfig({}, { mode: "development" });
}
// Do not set this, webpack-karma automatically unsets and emits a WARNING if
// it is there.
webpackConfig.entry = undefined;
webpackConfig.output = undefined;
webpackConfig.mode = "development";

// Allows tests to import modules locally
webpackConfig.resolve = {
  modules: ["./node_modules", "./"],
};

module.exports = function (config) {
  config.set({
    // base path that will be used to resolve all patterns (eg. files, exclude)
    basePath: "",

    // let our tests run for 5 seconds. Karma will wait for a message to be sent
    // over a websocket for this amount of time. If no message is received,
    // IE the browser is doing something else like running tests it will attempt
    // to reconnect one more time.
    browserDisconnectTimeout: 5000,

    // frameworks to use
    // available frameworks: https://npmjs.org/browse/keyword/karma-adapter
    frameworks: ["jasmine", "webpack"],

    // list of files / patterns to load in the browser
    files: [
      "node_modules/@webcomponents/custom-elements/custom-elements.min.js",
      "modules/**/*_test.js",
    ],

    // list of files / patterns to exclude
    exclude: [],

    plugins: [
      "karma-concat-preprocessor",
      "karma-webpack",
      "karma-jasmine",
      "karma-firefox-launcher",
      "karma-chrome-launcher",
      "karma-spec-reporter",
    ],

    // preprocess matching files before serving them to the browser
    // available preprocessors: https://npmjs.org/browse/keyword/karma-preprocessor
    preprocessors: {
      "modules/**/*_test.js": ["webpack"],
    },

    // test results reporter to use
    // possible values: 'dots', 'progress'
    // available reporters: https://npmjs.org/browse/keyword/karma-reporter
    reporters: ["spec"],

    // web server port
    port: parseInt(process.env.KARMA_PORT || "9876"),

    // enable / disable colors in the output (reporters and logs)
    colors: true,

    // level of logging
    // possible values: config.LOG_DISABLE || config.LOG_ERROR || config.LOG_WARN || config.LOG_INFO || config.LOG_DEBUG
    logLevel: config.LOG_INFO,

    // enable / disable watching file and executing tests whenever any file changes
    autoWatch: false,

    // start these browsers
    // available browser launchers: https://npmjs.org/browse/keyword/karma-launcher
    browsers: ["ChromeHeadless"],

    // Continuous Integration mode
    // if true, Karma captures browsers, runs the tests and exits
    singleRun: true,

    // Concurrency level
    // how many browser should be started simultaneous
    concurrency: Infinity,

    webpack: webpackConfig,
  });
};
