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
      return `<a href="${link.URL}" target="_blank">${link.Label}</a>`;
    }

    function groupTemplater(group, element, data) {
      let content = `
        <div class="group-title ${group.data.statusClassName}">
          ${group.data.label}
          <span class="duration">( ${group.data.duration} )</span>
        </div>`;
      if (group.data.text && group.data.text.length > 0) {
        content += group.data.text.join('<br>');
      }
      const links = [];
      // MainLink is an array of Links, SubLink is an array of array of Links.
      if (group.data.mainLink) {
        links.push(...group.data.mainLink);
      }
      if (group.data.subLink) {
        group.data.subLink.forEach(linkSet => links.push(...linkSet));
      }
      if (links.length > 0) {
        content += `<ul><li>${links.map(linkHtml).join('</li><li>')}</li></ul>`;
      } else {
        content += '<ul><li>- no logs -</li></ul>';
      }

      return content;
    }

    const options = {
      clickToUse: false,
      groupTemplate: groupTemplater,
      multiselect: false,
      onInitialDrawComplete: () => $('#timeline-rendering').remove(),
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
      if (item.data && item.data.mainLink && item.data.mainLink.length > 0) {
        window.open(item.data.mainLink[0].URL, '_blank');
      }
    });

    return timeline;
  }

  // Switches the view to a mode where overview, properties, and changes go on
  // one tab and the timeline goes on a second tab. This is intended to be
  // easier to read on a wide screen and waste less horizontal space.
  function goWideMode() {
    // Add the colummn class back to the overview, properties, and changes
    // divs and move the properties and changes divs to the "overview" tab.
    $('#overview').addClass('column');
    $('#overview-tab')
        .append($('#properties').addClass('column'))
        .append($('#changes').addClass('column'));
    // Remove the "Properties" and "Changes" tabs. Note that the corresponding
    // divs are left in the dom but will now be inaccessible.
    $('#tabs > ul > li > a[href="#properties-tab"]').parent().remove();
    $('#tabs > ul > li > a[href="#changes-tab"]').parent().remove();
  }

  // Narrow mode is the default, switch if necessary.
  if ($(window).width() > 1440 && useTabs) {
    goWideMode();
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
