// Copyright 2018 The LUCI Authors.
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

// A Series of time based utilites for the LogDog plaintext viewer.
// Requires: moment.js, moment-timezone.js, jquery, jquery-ui

// This file cannot use EcmaScript6: crbug.com/791528

(function(window) {
  'use strict';

  const tz = moment.tz.guess();

  /**
   * Given a Date, return a time string in the user's local timezone.
   * Also return the time string in relative time from now, MTV time, and UTC
   * time.
   */
  function formatDate(t) {
    var mt = moment.tz(t, tz);
    if (!mt.isValid()) {
      return null;
    }

    return {
      localLong: mt.format("YYYY-MM-DD LTS (z)"),
      MTVLong: moment.tz(mt, "America/Los_Angeles").format(
        "YYYY-MM-DD LTS [(MTV)]"),
      UTCLong: moment.tz(mt, "UTC").format("YYYY-MM-DD LTS [(UTC)]"),
    }
  }

  /***
   * Add local time information as a tooltip.
   */
  function formatTime(node) {
    if (node.title != "") return;  // Already done.
    try {
      var timestamp = node.getAttribute('data-timestamp');
      // Delta since last log line, in milliseconds.
      var deltaS = node.getAttribute('data-delta');
      var delta = parseInt(deltaS, 10) / 1000.0;
      var date = new Date(parseInt(timestamp, 10));
      var newTimestamp = formatDate(date);
      if (newTimestamp != null) {
        node.setAttribute(
          "title", [
            delta.toFixed(2) + "s since last line",
            newTimestamp.localLong,
            newTimestamp.MTVLong,
            newTimestamp.UTCLong,
          ].join("\n")
        )
      }
    } catch (e) {
      console.error('could not convert time of node', node, 'to local:', e)
    }
  }

  function setLocale() {
    // Moment.js does not set the locale automatically, it must be done by the
    // caller.
    var locale = window.navigator.userLanguage || window.navigator.language;
    moment.locale(locale);
  }

  // Export all methods and attributes as module level functions.
  Object.assign(window.utils = window.utils|| {}, {
    formatTime: formatTime,
    setLocale: setLocale,
  });

}(window));
