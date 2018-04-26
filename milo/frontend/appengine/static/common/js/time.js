// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// A Series of time based utilites for Milo.
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
      local: mt.format("YYYY-MM-DD LT (z)"),
      localLong: mt.format("YYYY-MM-DD LTS (z)"),
      MTVLong: moment.tz(mt, "America/Los_Angeles").format(
        "YYYY-MM-DD LTS [(MTV)]"),
      UTCLong: moment.tz(mt, "UTC").format("YYYY-MM-DD LTS [(UTC)]"),
      fromNow: mt.fromNow(),
    }
  }

  /**
   * Given two Dates (or a Date and null, prepresenting "now"), return
   * a duration string, with a hover string of the start/end time in the user's
   * timezone and locale.
   */
  function formatDuration(start, end) {
    var st = moment.tz(start, tz);
    if (!st.isValid()) {
      return null;
    }
    var hover = st.format("[Started: ] YYYY-MM-DD LTS (z)");
    hover += "\nEnded: ";
    if (end == null) {
      hover += "N/A";
    } else {
      var et =  moment.tz(end, tz);
      if (!et.isValid()) {
        return null
      }
      hover += et.format("YYYY-MM-DD LTS (z)");
    }
    return hover;
  }

  function makeTimesLocal(locale) {
    // Moment.js does not set the locale automatically, it must be done by the
    // caller.
    locale = locale || window.navigator.userLanguage || window.navigator.language;
    moment.locale(locale);

    var timeSpans = document.getElementsByClassName('local-time');
    for (var i = 0; i < timeSpans.length; i++) {
      var span = timeSpans[i];
      try {
        var oldTimestamp = span.innerText;
        var timestamp = span.getAttribute('data-timestamp');
        var date = new Date(parseInt(timestamp, 10));
        var newTimestamp = formatDate(date);
        if (newTimestamp != null) {
          span.setAttribute(
            "title", [
              newTimestamp.fromNow,
              newTimestamp.localLong,
              newTimestamp.MTVLong,
              newTimestamp.UTCLong,
            ].join("\n")
          )
          if (!$(span).hasClass('tooltip-only'))
            span.innerText = newTimestamp.local;
        }
      } catch (e) {
        console.error('could not convert time of span', span, 'to local:', e)
      }
    }
  }

  function annotateDurations() {
    var durations = document.getElementsByClassName('duration');
    for (var i = 0; i < durations.length; i++) {
      var dur = durations[i];
      try {
        var start = dur.getAttribute('data-starttime');
        var end = dur.getAttribute('data-endtime');
        var hover = formatDuration(start, end);
        if (hover != null) {
          dur.setAttribute("title", hover);
        }
      } catch (e) {
        console.error('could not annotate duration', dur, e)
      }
    }
  }

  // Export all methods and attributes as module level functions.
  Object.assign(window.milo = window.milo || {}, {
    makeTimesLocal: makeTimesLocal,
    annotateDurations: annotateDurations
  });

}(window));
