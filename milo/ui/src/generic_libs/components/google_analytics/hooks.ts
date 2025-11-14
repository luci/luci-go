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

import { useCallback } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';

import { useContentGroup } from './content_group';

declare global {
  interface Window {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    gtag?: (...args: any[]) => void;
  }
}

/**
 * The payload for a custom event.
 * Even though GA4 will accept any property name, this structure specifies properties for
 * type safety of callers, which should improve data quality.
 *
 * You can feel free to add as many properties as you like here for your custom events, just be
 * sure to also add them as custom properties in the GA4 UI under Admin > Custom definitions.
 */
export interface EventPayload {
  /**
   * The LUCI project associated with the event.
   */
  project?: string;

  /**
   * Whether the invocation/build that the event is associated with is presubmit or postsubmit.
   */
  invocationType?: 'presubmit' | 'postsubmit';

  /**
   * The name of the active tab on the page when the event is sent.
   */
  activeTab?: string;

  /** The name of the feature flag being opted in or out. */
  featureFlag?: string;

  /** The name of the component that was acted upon by the user.
   * e.g. the name of the link that was clicked or the button that was pressed.
   */
  componentName?: string;

  /**
   * The number of DUTs selected for an action.
   */
  selectedDuts?: number;

  /**
   * The kind of content being copied.
   */
  copyKind?: string;
}

/**
 * A hook that provides a function to track a custom analytics event.
 *
 * Example usage:
 * const trackEvent = useGoogleAnalytics();
 * useEffect(() => trackEvent({action: 'my_event', property1: value1}), [trackEvent, value1]);
 */
export function useGoogleAnalytics() {
  const contentGroup = useContentGroup();
  const { email } = useAuthState();

  const trackEvent = useCallback(
    (eventName: string, payload: EventPayload) => {
      if (!window.gtag) {
        // eslint-disable-next-line no-console
        console.warn(
          'Google Analytics not initialized. `trackEvent` was called but did nothing.',
          payload,
        );
        return;
      }

      if (!eventName || typeof eventName !== 'string') {
        // eslint-disable-next-line no-console
        console.warn(
          'GA Event Skipped: Invalid or missing `action` in payload.',
          payload,
        );
        return;
      }

      const eventParams: {
        [key: string]: string | boolean | number | undefined;
      } = {
        ...payload,
        category: contentGroup,
        isGoogler: email?.endsWith('@google.com'),
      };

      window.gtag('event', eventName, eventParams);
    },
    [contentGroup, email],
  );

  return { trackEvent };
}
