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

import { useContext, useEffect, useRef } from 'react';

import { UiPage } from '@/common/constants/view';

import { PageMetaStateCtx, PageMetaDispatcherCtx } from './context';

/**
 * Mark the component with a page identifier. When the component is mounted,
 * mark the page ID as activated.
 *
 * At most one page can be activated at a time.
 */
export function useDeclarePageId(pageId: UiPage) {
  const dispatch = useContext(PageMetaDispatcherCtx);
  if (dispatch === undefined) {
    throw new Error('useDeclarePageId can only be used in a PageMetaProvider');
  }

  const hookId = useRef();
  useEffect(() => {
    dispatch({ type: 'activatePage', pageId, hookId });
    return () => dispatch({ type: 'deactivatePage', pageId, hookId });
  }, [dispatch, pageId]);
}

export function useActivePageId() {
  const state = useContext(PageMetaStateCtx);
  if (state === undefined) {
    throw new Error('useActivePageId can only be used in a PageMetaProvider');
  }

  return state.activePage?.pageId;
}

/**
 * Set the project context to the specified value.
 *
 * Noop when `project` is undefined (i.e. the previous value is kept).
 */
export function useEstablishProjectCtx(project: string | undefined) {
  const dispatch = useContext(PageMetaDispatcherCtx);
  if (dispatch === undefined) {
    throw new Error(
      'useEstablishProjectCtx can only be used in a PageMetaProvider',
    );
  }

  useEffect(() => {
    if (!project) {
      return;
    }

    dispatch({ type: 'setProject', project });
  }, [dispatch, project]);
}

export function useProjectCtx() {
  const state = useContext(PageMetaStateCtx);
  if (state === undefined) {
    throw new Error('useProjectCtx can only be used in a PageMetaProvider');
  }

  return state.project;
}
