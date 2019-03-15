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

(function () {
  'use strict';
  (function(i,s,o,g,r,a,m){i['CrDXObject']=r;i[r]=i[r]||function(){
  (i[r].q=i[r].q||[]).push(arguments)},a=s.createElement(o),
  m=s.getElementsByTagName(o)[0];a.async=1;a.src=g;m.parentNode.insertBefore(a,m)
  })(window,document,'script','https://storage.googleapis.com/crdx-feedback.appspot.com/feedback.js','crdx');
  crdx('setFeedbackButtonLink', 'https://bugs.chromium.org/p/chromium/issues/entry?components=Infra%3EPlatform%3ELogDog');

  utils.setLocale();
  $(document).tooltip({
    show: false,
    hide: false
  });

  function setMode(mode) {
    document.cookie = `html-mode=${mode}; path=/`;
    location.reload(true);  // Changing modes require a force refresh.
  }

  $("#to-full").click(() => setMode("full"));
  $("#to-lite").click(() => setMode("lite"));
}());
