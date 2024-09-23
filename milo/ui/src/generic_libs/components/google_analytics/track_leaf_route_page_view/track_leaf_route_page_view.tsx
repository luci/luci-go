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

import { ReactNode } from 'react';

import { ContentGroup } from '../content_group';
import { PageViewTracker } from '../page_view_tracker';
import { TrackSearchParamKeys } from '../track_search_param_keys';

export interface TrackLeafRoutePageViewProps {
  /**
   * The content group of this route.
   *
   * This will be appended to the content group defined in the ancestor tree (if
   * any).
   */
  readonly contentGroup: string;
  /**
   * The search param keys to track.
   *
   * This will be added to the set of tracked search param keys defined by the
   * ancestors (if any).
   *
   * Updates to untracked search param keys will not trigger a new page view
   * event. This is designed to prevent the tracker from generating excessive
   * amount of page view events when frequently updated state (e.g. filters,
   * toggles) are stored in the search param.
   *
   * Untracked search param keys will not be sent to GA. This is designed to
   * reduce the chance of recording PII in GA. Recording PII in GA is forbidden
   * by our policy. See http://go/ooga-config.
   */
  readonly searchParamKeys?: readonly string[];
  readonly children: ReactNode;
}

/**
 * Tracks the page view event on a leaf route component.
 *
 * Note that nested `TrackLeafRoutePageView` should not be used. You should only
 * wrap the leaf routes you want to track with `TrackLeafRoutePageView`s.
 */
export function TrackLeafRoutePageView({
  contentGroup,
  searchParamKeys = [],
  children,
}: TrackLeafRoutePageViewProps) {
  return (
    <TrackSearchParamKeys keys={searchParamKeys}>
      <ContentGroup group={contentGroup}>
        <PageViewTracker>{children}</PageViewTracker>
      </ContentGroup>
    </TrackSearchParamKeys>
  );
}
