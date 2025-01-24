// Copyright 2024 The LUCI Authors.
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

import { ReactNode, useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';

import { logging } from '@/common/tools/logging';
import { useIsDevBuild } from '@/generic_libs/hooks/is_dev_build';

import { useContentGroup } from '../content_group';
import { useTrackedSearchParamKeys } from '../track_search_param_keys';

import { IsTrackedProvider, useIsTracked } from './context';

const NESTED_TRACKER_ERR_MEG = `
detected a nested PageViewTracker;\n
this is unsupported;\n
using a nested PageViewTracker is NOOP in production
`;

export interface PageViewTrackerProps {
  readonly children: ReactNode;
}

/**
 * Tracks the page view event on a route component.
 *
 * Note that nested `PageViewTracker` should not be used. You should only wrap
 * the leaf routes you want to track with `PageViewTracker`s.
 */
export function PageViewTracker({ children }: PageViewTrackerProps) {
  const isTracked = useIsTracked();
  const isDevBuild = useIsDevBuild();
  const loggedError = useRef(false);
  if (isTracked) {
    // Throw the error in during local development so it gets noticed and fixed.
    if (isDevBuild) {
      throw new Error(NESTED_TRACKER_ERR_MEG);
    }

    // When deployed, just log the error once. GA tracking misconfiguration
    // should not be a catastrophic error.
    if (!loggedError.current) {
      logging.error(NESTED_TRACKER_ERR_MEG);
      loggedError.current = true;
    }
  }

  const contentGroup = useContentGroup();
  const paramKeys = useTrackedSearchParamKeys();

  // Obtain a normalized URL (i.e. with untracked keys removed from the search
  // string.
  const location = useLocation();
  const normalizedSearchParams = new URLSearchParams(location.search);
  for (const key of normalizedSearchParams.keys()) {
    if (paramKeys.includes(key)) {
      continue;
    }
    normalizedSearchParams.delete(key);
  }
  const normalizedSearchStr = normalizedSearchParams.toString();
  let normalizedLocation = self.location.origin + location.pathname;
  if (normalizedSearchStr) {
    normalizedLocation += '?' + normalizedSearchStr;
  }

  useEffect(() => {
    // Don't emit page view event when there are nested trackers.
    // Otherwise we will have duplicated page_view events.
    if (isTracked) {
      return;
    }

    gtag('event', 'page_view', {
      page_title: document.title,
      // Only record the normalized location. This can reduce the chance of
      // recording PII in GA. Recording PII in GA is forbidden by our policy.
      // See http://go/ooga-config.
      page_location: normalizedLocation,
      content_group: contentGroup,
    });
  }, [
    isTracked,
    contentGroup,
    // Only fire an event when the URL updates after removing untracked search
    // param keys.
    //
    // This is designed to prevent the tracker from generating excessive
    // amount of page view events when frequently updated state (e.g. filters,
    // toggles) are stored in the search param.
    normalizedLocation,
  ]);

  return <IsTrackedProvider value={true}>{children}</IsTrackedProvider>;
}
