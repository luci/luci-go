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

import { UiPage } from '@/common/constants/view';

export interface PageMetaState {
  readonly activePage: {
    readonly pageId: UiPage;
    readonly hookId: unknown;
  } | null;
  readonly project: string | undefined;
}

export type Action = PageAction | ProjectAction;

export interface PageAction {
  readonly type: 'activatePage' | 'deactivatePage';
  /**
   * The ID of the page. Used to inform which page is current selected (e.g.
   * therefore needs to be highlighted in the navigation menu).
   */
  readonly pageId: UiPage;
  /**
   * An object that uniquely identifies a `usePageId` call in a component.
   *
   * Only one `usePageId` call can exist at any time.
   */
  readonly hookId: unknown;
}

export interface ProjectAction {
  readonly type: 'setProject';
  readonly project: string;
}

export function reducer(state: PageMetaState, action: Action): PageMetaState {
  switch (action.type) {
    case 'activatePage': {
      if (
        state.activePage !== null &&
        state.activePage.hookId !== action.hookId
      ) {
        throw new Error(
          `cannot activate a page when there's already an active page; ` +
            `active: ${state.activePage.pageId}}; ` +
            `activating: ${action.pageId}`,
        );
      }
      return {
        ...state,
        activePage: { pageId: action.pageId, hookId: action.hookId },
      };
    }
    case 'deactivatePage': {
      if (state.activePage?.hookId !== action.hookId) {
        throw new Error(
          `cannot deactivate an inactive tab; ` +
            `active: ${state.activePage?.pageId ?? 'NO_ACTIVE_PAGE'}; ` +
            `deactivating: ${action.pageId}`,
        );
      }
      return {
        ...state,
        activePage: null,
      };
    }
    case 'setProject': {
      return {
        ...state,
        project: action.project,
      };
    }
  }
}
