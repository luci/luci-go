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
 * pager. See the design decisions section in doc.md for details.
 */

import { useState } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
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
   *
   * If you use a custom pager component implementation, the custom pager
   * component needs to call this function when appropriate.
   */
  readonly onPrevPage?: () => void;
  /**
   * Called when user navigates to the next page *in the current browser tab*.
   *
   * This is useful for performing some post navigation option (e.g. scroll to
   * the top of the page).
   *
   * If you use a custom pager component implementation, the custom pager
   * component needs to call this function when appropriate.
   */
  readonly onNextPage?: () => void;
  /**
   * Called when user changes the page size *in the current browser tab*.
   *
   * This is useful for performing some post navigation option (e.g. scroll to
   * the top of the page).
   *
   * If you use a custom pager component implementation, the custom pager
   * component needs to call this function when appropriate.
   */
  readonly onPageSizeUpdated?: () => void;
}

// DO NOT EXPORT OUTSIDE OF param_pager module.
//
// So the state is private to the params_pager module.
export interface PagerState {
  /**
   * Hold the previous page tokens and the current page token.
   *
   * `pageTokens[i]` is the page token for `i+1`th page.
   *
   * There could be a lot of previous pages. Do not keep those tokens in the
   * URL.
   */
  readonly pageTokens: string[];
}

// DO NOT EXPORT.
//
// Use an unexported symbol so the property is private to the params_pager
// module.
const stateSymbol = Symbol('pagerState');

export interface PagerContext {
  /**
   * Options used to construct this pager context, with default values
   * populated.
   */
  readonly options: NonNullableProps<
    PagerOptions,
    'pageSizeKey' | 'pageTokenKey'
  >;
  readonly [stateSymbol]: PagerState;
}

export function usePagerContext(options: PagerOptions): PagerContext {
  const [searchParams] = useSyncedSearchParams();

  // This state does not actually affect rendering. It's used as an immutable
  // ref. We use `useState` instead of `useRef` here so we don't accidentally
  // assign value to `.current`.
  //
  // Mutation to the array is allowed.
  // Mutation to the reference to the array is not allowed.
  //
  // This ensures we always have a stable reference to the pageTokens array
  // while having the ability to update it without triggering a rerender.
  const [pageTokens] = useState(['']);

  const pagerCtx = {
    [stateSymbol]: {
      pageTokens,
    },
    options: {
      ...options,
      pageSizeKey: options.pageSizeKey || 'limit',
      pageTokenKey: options.pageTokenKey || 'cursor',
    },
  };

  // In theory, we don't need to use those getters because we have access to the
  // internals of the `pagerCtx` directly here. But use those getters anyway
  // just for consistency.
  const pageToken = getPageToken(pagerCtx, searchParams);

  // Keep the `pageTokens` house keeping here so we don't need to rely on any
  // pager component implementation to perform the house keeping work. This
  // allows this package to be used in a headless way.
  const pageTokenIndex = pageTokens.indexOf(pageToken);
  if (pageTokenIndex === -1) {
    // If the new page token is not found, assume it's a new page.
    // Record it in the page token array.
    pageTokens.push(pageToken);
  } else {
    // If the new page token is found, discard all the page tokens after it.
    pageTokens.splice(pageTokenIndex + 1);
  }

  return pagerCtx;
}

export function getPageSize(pagerCtx: PagerContext, params: URLSearchParams) {
  return (
    Number(params.get(pagerCtx.options.pageSizeKey)) ||
    pagerCtx.options.defaultPageSize
  );
}

export function pageSizeUpdater(pagerCtx: PagerContext, newPageSize: number) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (getPageSize(pagerCtx, searchParams) === newPageSize) {
      // Page isn't changed, so do nothing.
    } else if (newPageSize === pagerCtx.options.defaultPageSize) {
      searchParams.delete(pagerCtx.options.pageSizeKey);
    } else {
      searchParams.set(pagerCtx.options.pageSizeKey, String(newPageSize));
    }
    return searchParams;
  };
}

export function getPageToken(pagerCtx: PagerContext, params: URLSearchParams) {
  return params.get(pagerCtx.options.pageTokenKey) || '';
}

/**
 * Get the page token for the previous page.
 *
 * Returns an empty string if the previous page is the first page.
 * Returns null if there's no previous page.
 */
export function getPrevPageToken(pagerCtx: PagerContext) {
  return pagerCtx[stateSymbol].pageTokens.at(-2) ?? null;
}

export function pageTokenUpdater(pagerCtx: PagerContext, newPageToken: string) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (!newPageToken) {
      searchParams.delete(pagerCtx.options.pageTokenKey);
    } else {
      searchParams.set(pagerCtx.options.pageTokenKey, newPageToken);
    }
    return searchParams;
  };
}

export function getCurrentPageIndex(pagerCtx: PagerContext) {
  return pagerCtx[stateSymbol].pageTokens.length - 1;
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

export function prevPageTokenUpdater(pagerCtx: PagerContext) {
  return pageTokenUpdater(pagerCtx, getPrevPageToken(pagerCtx) || '');
}

export function nextPageTokenUpdater(
  pagerCtx: PagerContext,
  nextPageToken: string,
) {
  return pageTokenUpdater(pagerCtx, nextPageToken);
}
