// Copyright 2025 The LUCI Authors.
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
async function populate(url) {
  let text;
  try {
    const response = await fetch(url);
    if (response.ok) {
      text = await response.text();
    } else {
      text = `HTTP error! status: ${response.status}`;
    }
  } catch (error) {
    text = `Error fetching data: ${error}`;
  }
  const preEl = document.getElementById('logs-pre');
  preEl.textContent = text;
}

const urlHash = window.location.hash.substr(1)
// Manually parse the query string to avoid URLSearchParams converting '+' to spaces
const idx = urlHash.indexOf('url=');
if (idx !== -1) {
  const logUrl = urlHash.substr(idx + 4);
  populate(`/logs${logUrl}?format=raw`);
}
