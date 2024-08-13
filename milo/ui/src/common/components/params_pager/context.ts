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

/**
 * @fileoverview
 *
 * Pager context is the mechanism to hold state associated with a (set of) param
 * pager.
 *
 * Design decisions:
 * 1. Force the pager context to be constructed via a hook.
 *    This allows us to enforce that the state is always created and persisted
 *    when the pager is used. `eslint(react-hooks/rules-of-hooks)` will enforce
 *    that the state is not accidentally unloaded. If we hold the state in the
 *    pager component, users may unload the pager therefore accidentally discard
 *    the previous page tokens state when rendering a loading spinner.
 * 2. Force all page token/size getters/setters to take a pager context.
 *    This encourage users to lift the `usePagerContext` call to the top level
 *    component under which the pager and the state (e.g. page size, page token)
 *    it hold is used. As long as the pager is used in any form in the
 *    component, the pager state will not be discarded, even when the pager
 *    component itself is not rendered.
 * 3. Hold most pager configuration in the pager context.
 *    This ensures the pager component and the pager state getters/setters uses
 *    the same set of configuration. If different sets of configurations are
 *    used (e.g. different default page sizes are passed to `getPageSize` and
 *    `<ParamPager />`), the state may appear to be inconsistent (e.g. the
 *    page size used in the query is 50 but the highlighted selected page size
 *    is 25). Some pager configuration (e.g. `nextPageToken`) are not hold in
 *    the context because they may have not been initialized when pager context
 *    is constructed.
 */

import { useRef } from 'react';

import { NonNullableProps } from '@/generic_libs/types';

export interface PagerOptions {
  /**
   * A list of page sizes that users can choose from.
   */
  readonly pageSizeOptions: readonly number[];
  /**
   * The default page size.
   */
  readonly defaultPageSize: number;
  /**
   * URL search parameter key for page size.
   * Default to 'limit' when undefined or empty string is provided.
   */
  readonly pageSizeKey?: string;
  /**
   * URL search parameter key for page token.
   * Default to 'cursor' when undefined or empty string is provided.
   */
  readonly pageTokenKey?: string;
  /**
   * Called when user navigates to the previous page *in the current browser
   * tab*.
   *
   * This is useful for performing some post navigation option (e.g. scroll to
   * the top of the page).
   */
  readonly onPrevPage?: () => void;
  /**
   * Called when user navigates to the next page *in the current browser tab*.
   *
   * This is useful for performing some post navigation option (e.g. scroll to
   * the top of the page).
   */
  readonly onNextPage?: () => void;
  /**
   * Called when user changes the page size *in the current browser tab*.
   *
   * This is useful for performing some post navigation option (e.g. scroll to
   * the top of the page).
   */
  readonly onPageSizeUpdated?: () => void;
}

// DO NOT EXPORT OUTSIDE OF param_pager module.
//
// So the state is private to the params_pager module.
export interface PagerState
  extends NonNullableProps<PagerOptions, 'pageSizeKey' | 'pageTokenKey'> {
  /**
   * Hold the previous page tokens.
   *
   * There could be a lot of previous pages. Do not keep those tokens in the
   * URL.
   */
  readonly prevTokens: string[];
}

// DO NOT EXPORT.
//
// Use an unexported symbol so the property is private to the params_pager
// module.
const stateSymbol = Symbol('pagerState');

export interface PagerContext {
  readonly [stateSymbol]: PagerState;
}

export function usePagerContext(options: PagerOptions): PagerContext {
  const prevTokens = useRef<string[]>([]);
  return {
    [stateSymbol]: {
      ...options,
      pageSizeKey: options.pageSizeKey || 'limit',
      pageTokenKey: options.pageTokenKey || 'cursor',
      prevTokens: prevTokens.current,
    },
  };
}

// DO NOT EXPORT OUTSIDE OF param_pager module.
//
// So the state is private to the params_pager module.
export function getState(pagerCtx: PagerContext) {
  return pagerCtx[stateSymbol];
}

export function getPageSize(pagerCtx: PagerContext, params: URLSearchParams) {
  return (
    Number(params.get(pagerCtx[stateSymbol].pageSizeKey)) ||
    pagerCtx[stateSymbol].defaultPageSize
  );
}

export function pageSizeUpdater(pagerCtx: PagerContext, newPageSize: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newPageSize === pagerCtx[stateSymbol].defaultPageSize) {
      searchParams.delete(pagerCtx[stateSymbol].pageSizeKey);
    } else {
      searchParams.set(pagerCtx[stateSymbol].pageSizeKey, String(newPageSize));
    }
    return searchParams;
  };
}

export function getPageToken(pagerCtx: PagerContext, params: URLSearchParams) {
  return params.get(pagerCtx[stateSymbol].pageTokenKey) || '';
}

export function pageTokenUpdater(pagerCtx: PagerContext, newPageToken: string) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (!newPageToken) {
      searchParams.delete(pagerCtx[stateSymbol].pageTokenKey);
    } else {
      searchParams.set(pagerCtx[stateSymbol].pageTokenKey, newPageToken);
    }
    return searchParams;
  };
}

/**
 * Returns an updater that sets the page token to empty. This will also cause
 * all the previous page tokens to be discarded.
 *
 * Use this updater when the pagination context changes (e.g. when the filter
 * changes and the pages tokens are no longer valid). See
 * https://google.aip.dev/158 for details.
 */
export function emptyPageTokenUpdater(pagerCtx: PagerContext) {
  return pageTokenUpdater(pagerCtx, '');
}
