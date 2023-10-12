// Copyright 2023 The LUCI Authors.
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

// Code for managing the opt-in banner for the new builder page.
// TODO(weiweilin): remove this once we turned down the old builder page.

$(document).ready(function() {
  'use strict';

  function setCookie(name, value) {
    const farFuture = new Date(2100, 0).toUTCString();
    document.cookie = `${name}=${value}; expires=${farFuture}; path=/`;
  }

  const newBuilderPageLink = $('#new-builder-page-link');
  newBuilderPageLink.on('click', (e) => {
    if (!e.metaKey && !e.shiftKey && !e.ctrlKey && !e.altKey) {
      setCookie('showNewBuilderPage', true);
    }

    return true;
  });

  if (!document.cookie.includes('askForBuilderPageFeedback=false')) {
    $('#feedback-bar').toggleClass('show', true);
  }

  $('#dismiss-feedback-bar').click(e => {
    $('#feedback-bar').toggleClass('show', false);

    const expires = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toUTCString();
    document.cookie = `askForBuilderPageFeedback=false; expires=${expires}; path=/`;
    e.preventDefault();
  });

  $('#feedback-link').click(e => {
    const feedbackComment = encodeURIComponent(
`From Link: ${document.location.href}
Please enter a description of the problem, with repro steps if applicable.
`);
    const url = `https://bugs.chromium.org/p/chromium/issues/entry?template=Build%20Infrastructure&components=Infra%3ELUCI%3EUserInterface&labels=Pri-2,Type-Bug&comment=${feedbackComment}`;
    window.open(url);
    e.preventDefault();
  });

  // Keep the new builder page version up-to-date even when the user is using
  // the legacy builder page.
  navigator.serviceWorker?.register('/ui/ui_sw.js');
});
