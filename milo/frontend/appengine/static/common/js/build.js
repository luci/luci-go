// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code used for managing how nested steps are rendered in build view. In
// particular, it implement collapsing/nesting functionality and various modes
// for the Show option.

$(document).ready(function() {
  'use strict';

  const LEFT_TRIANGLE = '\u25B6';
  const DOWN_TRIANGLE = '\u25BC';

  $('li:has(>ol.substeps)').each(function() {
    let parentStepTitle = $(this).find('>div.result');
    let substepContainer = $(this).find('>ol.substeps');
    let collapsedIndicator = parentStepTitle.find('>div.nested-indicator');

    parentStepTitle.click(function() {
      substepContainer.toggleClass('collapsed');
      if (substepContainer.hasClass('collapsed')) {
        collapsedIndicator.text(LEFT_TRIANGLE);
      } else {
        collapsedIndicator.text(DOWN_TRIANGLE);
      }
    });
  });

  function updateCookieSetting(value) {
    document.cookie = 'buildShowPref=' + value + ';' +
      'expires=Fri Jan 01 2100 00:00:00 UTC; path=/';
  }

  $("#showExpanded").click(function(e) {
    $('ol.substeps').removeClass('collapsed');
    $('#steps').removeClass('non-green');
    $('li:has(>ol.substeps)>div.result>.nested-indicator').text(DOWN_TRIANGLE);
    updateCookieSetting('expanded');
  });

  $("#showCollapsed").click(function(e) {
    $('ol.substeps').addClass('collapsed');
    $('#steps').removeClass('non-green');
    $('li:has(>ol.substeps)>div.result>.nested-indicator').text(LEFT_TRIANGLE);
    updateCookieSetting('collapsed');
  });

  $("#showNonGreen").click(function(e) {
    $('ol.substeps').removeClass('collapsed');
    $('#steps').addClass('non-green');
    $('li:has(>ol.substeps)>div.result>.nested-indicator').text(DOWN_TRIANGLE);
    updateCookieSetting('non-green');
  });
});
