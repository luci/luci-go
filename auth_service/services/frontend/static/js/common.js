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

  // Returns URL to a change log page for a given target.
  exports.getChangeLogTargetURL = (kind, name) => {
    return '/change_log?target=' + encodeURIComponent(kind + '$' + name);
  };

  // Returns URL to a change log page for a given revision.
  exports.getChangeLogRevisionURL = (rev) => {
    return '/change_log?auth_db_rev=' + encodeURIComponent('' + rev);
  };

  // Returns value of URL query parameter given its name.
  exports.getQueryParameter = (name) => {
    // See http://stackoverflow.com/a/5158301.
    var match = new RegExp(
      '[?&]' + name + '=([^&]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
  };

  return exports;
})();