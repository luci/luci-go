/*
  Copyright 2016 The Chromium Authors. All rights reserved.
  Use of this source code is governed by a BSD-style license that can be
  found in the LICENSE file.
*/

'use strict';

var packageJsonFile = '../../package.json';

// Include Gulp & tools we'll use
var gulp = require('gulp');
var luci = require('../gulp-common.js');

luci.setup(gulp, {
  dir: __dirname,
});
