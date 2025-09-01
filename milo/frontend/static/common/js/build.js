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

  function setCookie(name, value) {
    const farFuture = new Date(2100, 0).toUTCString();
    document.cookie = `${name}=${value}; expires=${farFuture}; path=/`;
  }

  function displayPrefChanged(selectedOption) {
    if (!['expanded', 'default', 'non-green'].includes(selectedOption)) {
      selectedOption = 'default';
    }

    $('li.substeps').removeClass('collapsed');
    $('#steps').removeClass('non-green');

    switch (selectedOption) {
      case 'expanded':
        break;
      case 'default':
        $('li.substeps.green').addClass('collapsed');
        break;
      case 'non-green':
        $('#steps').addClass('non-green');
        break;
    }

    setCookie('stepDisplayPref', selectedOption);
  }

  const newBuildPageLink = $('#new-build-page-link');
  newBuildPageLink.on('click', (e) => {
    if (e.metaKey || e.shiftKey || e.ctrlKey || e.altKey) {
      return true;
    }

    setCookie('showNewBuildPage', true);
    return true;
  });

  displayPrefChanged($('input[name="hider"]:checked').val());
  $('input[name="hider"]').change(function() {displayPrefChanged($(this).val())});

  $('#showDebugLogs').change(function(e) {
    setCookie('showDebugLogsPref', this.checked)
    $('#steps').toggleClass('hide-debug-logs', !this.checked);
  });

  function createTimeline() {
    function groupTemplater(group, element, data) {
      return `
        <div class="group-title ${group.data.statusClassName}">
          <span class="title">${group.data.label}</span>
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
  let relatedBuilds = null;

  // By hiding the tab div until the tabs are constructed a flicker
  // of the tab contents not within the tabs is avoided.
  $('#tabs').tabs({
    activate: function(event, ui) {
      switch (ui.newPanel.attr('id')) {
        // By lazily creating the timeline only when its tab is first activated an
        // ugly multi-step rendering (and console warning of infinite redraw loop)
        // of the timeline is avoided. This could also be avoided by making the
        // timeline tab the initially visible tab and creating the timeline just
        // after the tabs are initialized (that is, wouldn't have to use this
        // activate event hack).
        case 'timeline-tab':
          if (timeline === null) {
            timeline = createTimeline();
          }
          break;

        case 'blamelist-tab':
          if (!blamelistLoaded) {
            const params = (new URL(window.location)).searchParams;
            params.set('blamelist', '1');
            document.location = `?${params.toString()}#blamelist-tab`;
          }
          break;

        // Load related builds asynchronously so it doesn't block rendering the rest of the page.
        case 'related-tab':
          const tableContainerRef = $('#related-builds-table-container')
          if (!relatedBuilds && tableContainerRef) {
            relatedBuilds = fetch(`/internal_widgets/related_builds/${buildbucketId}`)
              .then((res) => res.text())
              .then((tablehtml) => tableContainerRef.html(tablehtml))
              .catch((_) => tableContainerRef.html('<p style="color: red;">Something went wrong</p>'))
          }
          break;
      }
    },
  }).show();

  const closeModal = () => {
    $('#modal-overlay')
      .removeClass('show')
      .removeClass('show-retry-build-modal')
      .removeClass('show-cancel-build-modal');
  };

  $('#retry-build-button').click(e => {
    closeModal();
    $('#modal-overlay')
      .addClass('show')
      .addClass('show-retry-build-modal');
  });
  $('#dismiss-retry-build-button').click(closeModal);

  $('#cancel-build-button').click(e => {
    closeModal();
    $('#modal-overlay')
      .addClass('show')
      .addClass('show-cancel-build-modal');
  });
  $('#dismiss-cancel-build-button').click(closeModal);

  $('#modal-overlay').click(e => {
    if ($(e.target).is('#modal-overlay')) {
      closeModal();
    }
  });

  if (!document.cookie.includes('askForFeedback=false')) {
    $('#feedback-bar').toggleClass('show', true);
  }

  $('#dismiss-feedback-bar').click(e => {
    $('#feedback-bar').toggleClass('show', false);

    const expires = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toUTCString();
    document.cookie = `askForFeedback=false; expires=${expires}; path=/`;
    e.preventDefault();
  });

  $('#feedback-link').click(e => {
    const feedbackComment = encodeURIComponent(
`From Link: ${document.location.href}
Please enter a description of the problem, with steps to reproduce if applicable.
`);
    const url = `https://issuetracker.google.com/issues/new?component=1456503&type=BUG&priority=P2&severity=S2&inProd=true&description=${feedbackComment}`;
    window.open(url);
    e.preventDefault();
  });
});
