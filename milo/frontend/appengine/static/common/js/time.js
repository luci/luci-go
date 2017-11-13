// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// A Series of time based utilites for Milo.
// Requires: moment.js, moment-timezone.js, jquery, jquery-ui


(function(window) {
  'use strict';

  var milo = window.milo || {};

  milo.tz = moment.tz.guess();

  /**
   * Given a Date, return a time string in the user's local timezone.
   * Also return the time string in relative time from now, MTV time, and UTC
   * time.
   */
  milo.formatDate = function(t) {
    var mt = moment.tz(t, milo.tz);
    if (!mt.isValid()) {
      return null;
    }
    var hover = mt.fromNow();
    hover += "\n" + moment.tz(mt, "America/Los_Angeles").format("YYYY-MM-DD LT [(MTV)]");
    hover += "\n" + moment.tz(mt, "UTC").format("YYYY-MM-DD LT [(UTC)]");

    return {
      main: mt.format("YYYY-MM-DD LT (z)"),
      hover: hover
    }
  };

  /**
   * Given two Dates (or a Date and null, prepresenting "now"), return
   * a duration string, with a hover string of the start/end time in the user's
   * timezone and locale.
   */
  milo.formatDuration = function(start, end) {
    var st = moment.tz(start, milo.tz);
    if (!st.isValid()) {
      return null;
    }
    var hover = st.format("[Started: ] YYYY-MM-DD LT (z)")
    hover += "\nEnded: "
    if (end == null) {
      hover += "N/A";
    } else {
      var et =  moment.tz(end, milo.tz)
      if (!et.isValid()) {
        return null
      }
      hover += et.format("YYYY-MM-DD LT (z)");
    }
    return hover;
  };

  milo.makeTimesLocal = function(locale) {
    // Moment.js does not set the local automatically, it must be done by the
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
        var newTimestamp = milo.formatDate(date);
        if (newTimestamp != null) {
          span.innerText = newTimestamp.main;
          span.setAttribute("title", newTimestamp.hover);
        }
      } catch (e) {
        console.error('could not convert time of span', span, 'to local:', e)
      }
    }
  };

  milo.annotateDurations = function() {
    var durations = document.getElementsByClassName('duration');
    for (var i = 0; i < durations.length; i++) {
      var dur = durations[i];
      try {
        var start = dur.getAttribute('data-starttime');
        var end = dur.getAttribute('data-endtime');
        var hover = milo.formatDuration(start, end);
        if (hover != null) {
          dur.setAttribute("title", hover);
        }
      } catch (e) {
        console.error('could not annotate duration', dur, e)
      }
    }
  }

  window.milo = milo;

}(window));
