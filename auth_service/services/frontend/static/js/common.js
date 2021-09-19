// Copyright 2021 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

var common = (function () {
  'use strict';

  var exports = {};

  // Strips '<prefix>:' from a string if it starts with it.
  exports.stripPrefix = function (prefix, str) {
    if (!str) {
      return '';
    }
    if (str.slice(0, prefix.length + 1) == prefix + ':') {
      return str.slice(prefix.length + 1, str.length);
    } else {
      return str;
    }
  };

  return exports;
})();