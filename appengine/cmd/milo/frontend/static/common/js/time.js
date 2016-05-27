// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// A Series of time based utilites for Milo.


(function(window) {
  'use strict';

  var milo = window.milo || {};

  /**
   * Given a Javascript parsable time string, return a time string in the user's
   * local timezone
   */
  milo.formatDate = function(dt) {
    if (!dt) {
        return dt;
    }
    var shortDayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    var t = new Date(dt);
    var offset = -(new Date()).getTimezoneOffset();
    var offsetHr = Math.abs(Math.round(offset / 60));
    var offsetMin = Math.abs(Math.abs(offset) - (offsetHr * 60));
    if (offsetHr < 10) {
      offsetHr = '0' + offsetHr;
    }
    if (offsetMin < 10) {
      offsetMin = '0' + offsetMin;
    }
    var offsetStr = 'UTC';
    if (offset > 0) {
      offsetStr = '+' + offsetHr + ':' + offsetMin;
    } else if (offset < 0) {
      offsetStr = '-' + offsetHr + ':' + offsetMin;
    }

    var month = (t.getMonth() + 1);
    if (month < 10) {
      month = '0' + month;
    }
    var date = t.getDate();
    if (date < 10) {
      date = '0' + date;
    }
    var s = shortDayNames[t.getDay()] + ', ';
    s += t.getFullYear() + '-' + month + '-' + date + ' ';
    s += t.toLocaleTimeString();
    s += ' (' + offsetStr + ')';

    return s;
  };

  window.milo = milo;

}(window));
