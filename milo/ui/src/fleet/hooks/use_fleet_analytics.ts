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

import { useCallback, useMemo } from 'react';

import { platformToURL } from '@/fleet/constants/paths';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import {
  EventPayload,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';

export interface FleetEventPayload extends EventPayload {
  /** The ID of the table column acted upon. */
  columnId?: string;
  /** Whether the column is visible after the action. */
  visible?: boolean;
}

/**
 * Custom hook for Fleet Console tracking that automatically injects the current platform attribute into GA events.
 */
export function useFleetAnalytics() {
  const { trackEvent: baseTrackEvent } = useGoogleAnalytics();
  const currentPlatform = useCurrentPlatform();

  const trackEvent = useCallback(
    (eventName: string, payload?: FleetEventPayload) => {
      const platformStr =
        currentPlatform !== undefined
          ? platformToURL(currentPlatform)
          : undefined;

      baseTrackEvent(eventName, {
        platform: platformStr,
        ...payload,
      });
    },
    [baseTrackEvent, currentPlatform],
  );

  const trackExternalLinkClick = useCallback(
    (event: { target: EventTarget | null }) => {
      if (!(event.target instanceof Element)) {
        return;
      }
      const anchor = event.target.closest('a');
      if (!anchor) {
        return;
      }
      const rawUrl = anchor.getAttribute('href') || anchor.getAttribute('to');
      if (!rawUrl || !/^https?:\/\//i.test(rawUrl)) {
        return;
      }
      try {
        const parsed = new URL(rawUrl, window.location.origin);
        if (parsed.origin === window.location.origin) {
          return;
        }
      } catch {
        // Keep tracking even if a shorthand URL fails standard URL parsing.
      }
      trackEvent('external_link_clicked', {
        componentName: rawUrl,
      });
    },
    [trackEvent],
  );

  return useMemo(
    () => ({ trackEvent, trackExternalLinkClick }),
    [trackEvent, trackExternalLinkClick],
  );
}
