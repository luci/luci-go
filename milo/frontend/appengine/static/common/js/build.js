// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code used for managing how nested steps are rendered in build view. In
// particular, it implement collapsing/nesting functionality and various modes
// for the Show option.

$(document).ready(function() {
  'use strict';

  $('li.substeps').each(function() {
    const substep = $(this);
    $(this).find('>div.result').click(function() {
      substep.toggleClass('collapsed');
    });
  });

  function updateCookieSetting(value) {
    const farFuture = new Date(2100, 0).toUTCString();
    document.cookie = `stepDisplayPref=${value}; expires=${farFuture}; path=/`;
  }

  $('#showExpanded').click(function(e) {
    $('li.substeps').removeClass('collapsed');
    $('#steps').removeClass('non-green');
    updateCookieSetting('expanded');
  });

  $('#showDefault').click(function(e) {
    $('li.substeps').removeClass('collapsed');
    $('li.substeps.green').addClass('collapsed');
    $('#steps').removeClass('non-green');
    updateCookieSetting('default');
  });

  $('#showNonGreen').click(function(e) {
    $('li.substeps').removeClass('collapsed');
    $('#steps').addClass('non-green');
    updateCookieSetting('non-green');
  });
});
