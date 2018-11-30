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
    function linkHtml(link) {
      return '<a href="' + link.URL + '" target="_blank">' + link.Label + '</a>';
    }

    function groupTemplater(group, element, data) {
      var content = '<div class="group-title ' + group.data.statusClassName + '">' + group.data.label + '</div>';
      if (group.data.text && group.data.text.length > 0) {
        var texts = group.data.text.filter(function(text) {return text !== ''});
        content += '<ul><li>' + texts.join('</li><li>') + '</li></ul>';
      }
      const links = [];
      // MainLink is an array of Links, SubLink is an array of array of Links.
      if (group.data.mainLink && group.data.mainLink.length > 0) {
        links.push.apply(links, group.data.mainLink);
      }
      if (group.data.subLink && group.data.subLink.length > 0) {
        group.data.subLink.forEach(function(linkSet) {links.push.apply(links, linkSet)});
      }
      if (links.length > 0) {
        content += '<ul><li>' + links.map(function(link) {return linkHtml(link)}).join('</li><li>') +  '</li></ul>';
      } else {
        content += '<ul><li>- no logs -</li></ul>';
      }

      return content;
    }

    function itemTemplater(item, element, data) {
      return item.data.label;
    }

    const options = {
      clickToUse: false,
      groupTemplate: groupTemplater,
      multiselect: false,
      onInitialDrawComplete: function() {$('#rendering').remove()},
      orientation: {
        axis: 'both',
        item: 'top',
      },
      template: itemTemplater,
      zoomable: false,
    };

    const timeline = new vis.Timeline(document.getElementById('timeline'),
        new vis.DataSet(timelineData.Items), new vis.DataSet(timelineData.Groups),
        options);
    timeline.on('select', function(props) {
      const item = timeline.itemsData.get(props.items[0]);
      if (!item) {
        return;
      }
      if (item.data && item.data.mainLink && item.data.mainLink.length > 0) {
        window.open(item.data.mainLink[0].URL, '_blank');
      }
    });

    return timeline;
  }

  var wideMode = false;

  // Switches the view to a mode where results, properties, changes,
  // and timeline all get their own tab. This is intended to be easier
  // to read on a narrow screen.
  function goNarrowMode() {
    if (!wideMode) {
      return;
    }
    // Add the "Properties" and "Changes" tabs back in in the proper position.
    $('#tabs > ul > li > a[href="#results-tab"]').parent()
        .after($('<li><a href="#changes-tab">Changes</a></li>'))
        .after($('<li><a href="#properties-tab">Properties</a></li>'));
    // Remove the column class from the results, properties, and changes
    // divs and move the properties and changes divs back to their own tabs.
    $('#results').removeClass('column');
    $('#properties-tab').append($('#properties').removeClass('column'));
    $('#changes-tab').append($('#changes').removeClass('column'));
    wideMode = false;
  }

  // Switches the view to a mode where results, properties, and changes go on
  // one tab and the timeline goes on a second tab. This is intended to be
  // easier to read on a wide screen and waste less horizontal space.
  function goWideMode() {
    if (wideMode) {
      return;
    }
    // Add the colummn class back to the results, properties, and changes
    // divs and move the properties and changes divs to the "Results" tab.
    $('#results').addClass('column');
    $('#results-tab')
        .append($('#properties').addClass('column'))
        .append($('#changes').addClass('column'));
    // Remove the "Properties" and "Changes" tabs. Note that the corresponding
    // divs are left in the dom but will now be inaccessible.
    $('#tabs > ul > li > a[href="#properties-tab"]').parent().remove();
    $('#tabs > ul > li > a[href="#changes-tab"]').parent().remove();
    wideMode = true;
  }

  // Narrow mode is the default, switch if necessary.
  if ($(window).width() > 1440) {
    goWideMode();
  }

  var timeline = null;

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

  // To add an an interactive method of switching view modes react to an event
  // by calling goNarrowMode() or goWideMode() and then call
  // $('#tabs').tabs('refresh');
});
