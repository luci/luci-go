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

function reimport(config_set) {
  fetch('/internal/frontend/reimport/' + config_set, {
    method: 'POST',
    mode: "same-origin",
    credentials: "same-origin"
  }).then(function (response) {
    if (response.ok) {
      document.getElementById("reimport-modal-body").innerHTML = "successfully reimported"
    } else {
      response.text().then(text => {
        document.getElementById("reimport-modal-body").innerHTML = "failed to reimport. Reason: " + text
      })
    }
  }).catch(function (error) {
      console.error('Error: ', error)
    });
}

// reload the page after closing the reimport modal
document.getElementById("reimport-modal").addEventListener('hidden.bs.modal', function () {
  location.reload();
})

