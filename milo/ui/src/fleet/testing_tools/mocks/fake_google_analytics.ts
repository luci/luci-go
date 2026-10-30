// Copyright 2026 The LUCI Authors.
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

export interface TrackedGaEvent {
  eventName: string;
  payload: Record<string, unknown>;
}

/**
 * Installs a lightweight fake `window.gtag` implementation for tests so
 * components using `useGoogleAnalytics()` can be exercised without mocking the
 * analytics module itself. Restores the prior `window.gtag` state on `clear()`.
 */
export const fakeGoogleAnalytics = () => {
  const previousGtag = window.gtag;
  const hadOwnGtag = Object.prototype.hasOwnProperty.call(window, 'gtag');
  const events: TrackedGaEvent[] = [];
  const gtagSpy = jest.fn(
    (command: string, eventName: string, payload: Record<string, unknown>) => {
      if (command === 'event') {
        events.push({ eventName, payload });
      }
    },
  );

  window.gtag = gtagSpy as unknown as typeof window.gtag;

  return {
    gtagSpy,
    events,
    clear: () => {
      events.length = 0;
      gtagSpy.mockClear();
      if (hadOwnGtag) {
        window.gtag = previousGtag;
      } else {
        delete (window as { gtag?: unknown }).gtag;
      }
    },
  };
};
