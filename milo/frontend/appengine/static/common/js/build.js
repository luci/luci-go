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

  function createTimeline() {
    function groupTemplater(group, element, data) {
      return `
        <div class="group-title ${group.data.statusClassName}">
          ${group.data.label}
          <span class="duration">( ${group.data.duration} )</span>
        </div>`;
    }

    const options = {
      clickToUse: false,
      groupTemplate: groupTemplater,
      multiselect: false,
      onInitialDrawComplete: () => {
        $('#timeline-rendering').remove();
      },
      orientation: {
        axis: 'both',
        item: 'top',
      },
      template: item => item.data.label,
      zoomable: false,
    };

    const timeline = new vis.Timeline(
        document.getElementById('timeline'),
        new vis.DataSet(timelineData.items),
        new vis.DataSet(timelineData.groups),
        options);
    timeline.on('select', function(props) {
      const item = timeline.itemsData.get(props.items[0]);
      if (!item) {
        return;
      }
      if (item.data && item.data.logUrl) {
        window.open(item.data.logUrl, '_blank');
      }
    });

    return timeline;
  }

  let timeline = null;

  // By hiding the tab div until the tabs are constructed a flicker
  // of the tab contents not within the tabs is avoided.
  $('#tabs').tabs({
    activate: function(event, ui) {
      // By lazily creating the timeline only when its tab is first activated an
      // ugly multi-step rendering (and console warning of infinite redraw loop)
      // of the timeline is avoided. This could also be avoided by making the
      // timeline tab the initially visible tab and creating the timeline just
      // after the tabs are initialized (that is, wouldn't have to use this
      // activate event hack).
      if (ui.newPanel.attr('id') === 'timeline-tab' && timeline === null) {
        timeline = createTimeline();
      }
    },
  }).show();
});
