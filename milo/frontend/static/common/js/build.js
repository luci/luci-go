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

  function trackEvent(category, action, label) {
    ga('send', {
      hitType: 'event',
      eventCategory: category,
      eventAction: action,
      eventLabel: label,
      transport: 'beacon',
    });
  }

  if (!window.location.href.includes('javascript:')) {
    trackEvent('Old Build Page', 'Page Visited', window.location.href);
    const project = /\/p\/([^\/]+)\//.exec(window.location.href)?.[1] || 'unknown';
    trackEvent('Project Build Page', 'Visited Legacy', project)
  }

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
      trackEvent(
        'New Build Page',
        'Switch Version Temporarily',
        window.location.href,
      );
      return true;
    }

    trackEvent('New Build Page', 'Switch Version', window.location.href);
    setCookie('showNewBuildPage', true);

    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/root_sw.js')
        .then((registration) => {
          console.log('Root SW registered: ', registration);
          window.open(newBuildPageLink.attr('href'), newBuildPageLink.attr('target') || '_self');
        });
      return false;
    }
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
Please enter a description of the problem, with repro steps if applicable.
`);
    const url = `https://bugs.chromium.org/p/chromium/issues/entry?template=Build%20Infrastructure&components=Infra%3ELUCI%3EUserInterface%3EResultUI&labels=Pri-2,Type-Bug&comment=${feedbackComment}`;
    window.open(url);
    e.preventDefault();
  });

  setUpNewBuildPageSurvey();

  // Keep the new build page version up-to-date even when the user is using the
  // legacy build page.
  // Otherwise the new build page version will not take effect until the user
  // loads the new build page for the 2nd time due to service worker activation
  // rules.
  navigator.serviceWorker?.register('/ui/ui_sw.js')
    .then((registration) => registration.update())
    .then((registration) => {
      console.log('New build page SW registered: ', registration);
    });
});

/**
 * Asks users why they decided to opted out the new build page.
 */
function setUpNewBuildPageSurvey() {
  let apiKey = '';
  if (['luci-milo.appspot.com', 'ci.chromium.org'].includes(window.location.hostname)) {
    apiKey = 'AIzaSyDMYX77ySjK3Lib08BIjgvFn2Ur-rhAJvA';
  } else if (['luci-milo-dev.appspot.com'].includes(window.location.hostname)) {
    apiKey = 'AIzaSyD3f5TrWDSbkvWe2ZavVHg5QaLqsFnqZHE';
  }

  // Custom deployment, don't trigger feedback.
  if (!apiKey) {
    return;
  }

  const helpApi = window.help.service.Lazy.create(0, {apiKey, locale: 'en-US'});

  // New build page doesn't support raw builds, no point asking for feedback
  // here.
  if (/^\/raw\//.test(window.location.pathname)) {
    return;
  }

  const triggerId = /^\/old\//.test(window.location.pathname)
    ? 'hB2DSG5tV0py7BkUcZm0XboqSd3F'
    : 'WoXSyHEYJ0py7BkUcZm0Qk4cewfw';

  helpApi.requestSurvey({
    triggerId,
    callback: (requestSurveyCallbackParam) => {
      if (!requestSurveyCallbackParam.surveyData) {
        return;
      }

      // Hide the feedback button temporarily.
      document.querySelector('.__crdxFeedbackButton').style.display = 'none';

      helpApi.presentSurvey({
        surveyData: requestSurveyCallbackParam.surveyData,
        colorScheme: 1,
        customZIndex: 10000,
        listener: {
          surveyClosed: () => {
            // Show the feedback button again.
            document.querySelector('.__crdxFeedbackButton').style.display = '';
          },
          surveyPrompted: () => {
            // Remove title so the instant tooltip won't show up.
            document.querySelector('#google-hats-survey').title = '';
          }
        }
      });
    }
  });
}
