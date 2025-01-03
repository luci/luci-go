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

/**
 * Once the user switches back to the old version, persist the selection for one
 * week.
 */
const ROLLBACK_PERSIST_DURATION = 7 * 24 * 60 * 60 * 1000;

/**
 * Activate the old UI and reload the page.
 */
export async function switchToOldUI() {
  document.cookie = `USER_INITIATED_ROLLBACK=true; expires=${new Date(Date.now() + ROLLBACK_PERSIST_DURATION)}`;

  await reactivateUI();
}

/**
 * Activate the new UI and reload the page.
 */
export async function switchToNewUI() {
  document.cookie = `USER_INITIATED_ROLLBACK=false`;

  await reactivateUI();
}

async function reactivateUI() {
  // Unregister all the service workers so the new page load request will hit
  // the server and be served with another version.
  const registrations = await navigator.serviceWorker.getRegistrations();
  await Promise.allSettled(registrations.map((reg) => reg.unregister()));

  location.reload();
}
