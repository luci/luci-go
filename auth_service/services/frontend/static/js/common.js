// Copyright 2022 The LUCI Authors.
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

var common = (function () {
  'use strict';

  var exports = {};

  // Strips '<prefix>:' from a string if it starts with it.
  exports.stripPrefix = (prefix, str) => {
    if (!str) {
      return '';
    }
    if (str.slice(0, prefix.length + 1) == prefix + ':') {
      return str.slice(prefix.length + 1, str.length);
    } else {
      return str;
    }
  };

  // Converts UTC timestamp (in microseconds) to a readable string in local TZ.
  exports.utcTimestampToString = (utc) => {
    return (new Date(utc)).toLocaleString();
  };

  return exports;
})();