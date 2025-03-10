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

import { Duration } from 'luxon';

import { ROLLBACK_PERSIST_DURATION_WEEK } from './constants';

/**
 * The key of the cookie that controls user-initiated-rollback.
 *
 * Note that this should not be used to detect whether user is on the old
 * version. Because the page may not have been refreshed after the cookie is
 * updated.
 */
const ROLLBACK_COOKIE_KEY = 'USER_INITIATED_ROLLBACK';

/**
 * Activate the old UI and reload the page.
 */
export async function switchToOldUI() {
  const maxAge =
    Duration.fromObject({ week: ROLLBACK_PERSIST_DURATION_WEEK }).toMillis() /
    1000;
  document.cookie = `${ROLLBACK_COOKIE_KEY}=true; path=/; max-age=${maxAge}`;

  await reactivateUI();
}

/**
 * Activate the new UI and reload the page.
 */
export async function switchToNewUI() {
  document.cookie = `${ROLLBACK_COOKIE_KEY}=false; path=/`;

  await reactivateUI();
}

async function reactivateUI() {
  const registrations = await navigator.serviceWorker.getRegistrations();

  // Ensure all service workers are updated.
  await Promise.allSettled(registrations.map((reg) => reg.update()));

  // Unregister all service workers to ensure the first request (after reload)
  // is not intercepted by the service worker due to race conditions.
  await Promise.allSettled(registrations.map((reg) => reg.unregister()));

  // If the above doesn't work (e.g. when the service worker takes a long time
  // to update), we can use the same logic used to implement
  // `stale-while-revalidate` function of the service worker to mark the
  // current service worker as "outdated".

  location.reload();
}
