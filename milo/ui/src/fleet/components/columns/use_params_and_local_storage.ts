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

import _ from 'lodash';
import { useEffect, useState, useRef } from 'react';
import { useLocalStorage } from 'react-use';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { SetURLSearchParams } from '@/generic_libs/hooks/synced_search_params/context';

/**
 * Saves a state and keeps it in sync with both query parameters and local storage
 * The priority order for the source of truth is:
 *    query parameters -> local storage -> default value
 */
export const useParamsAndLocalStorage = (
  searchParamsKey: string,
  localStorageKey: string,
  defaultValue: string[],
): [
  string[],
  (new_value: string[] | ((prev: string[]) => string[])) => void,
] => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [localStorageState, setLocalStorageState, clearLocalStorage] =
    useLocalStorage<string[]>(localStorageKey);

  // We keep an internal state for optimistic updates
  const [syncedState, setSyncedState] = useState(
    getInitialValue(
      searchParams,
      searchParamsKey,
      localStorageState,
      defaultValue,
    ),
  );

  // Keep track of the last params we successfully processed from the URL/LocalStorage
  // This allows us to ignore "stale" updates where the URL hasn't actually changed yet
  const lastProcessedParams = useRef<string[] | undefined>(undefined);
  // Mirror of syncedState to allow access inside useEffect without adding
  // syncedState to the dependency array (which would cause infinite loops).
  const stateRef = useRef(syncedState);

  // Sync from URL/LocalStorage to internal state
  const currentParamsValue = getInitialValue(
    searchParams,
    searchParamsKey,
    localStorageState,
    defaultValue,
  );

  useEffect(() => {
    // Stringify for stable comparison
    const currentStr = JSON.stringify([...currentParamsValue].sort());
    const lastStr = lastProcessedParams.current
      ? JSON.stringify([...lastProcessedParams.current].sort())
      : undefined;

    // Check 1: Did the external source (URL) actually change?
    // This is crucial for ignoring "stale" re-renders where the URL hasn't updated yet,
    // but the component re-rendered (e.g. due to optimistic state change).
    if (currentStr !== lastStr) {
      // Check 2: Does the new external source match our current internal state?
      // If the user optimistically updated state, and the URL is stale (caught by Check 1), we skip this.
      // If the URL *did* change (e.g. back button), we want to sync our internal state to it.
      if (!_.isEqual(currentParamsValue, stateRef.current)) {
        setSyncedState(currentParamsValue);
        stateRef.current = currentParamsValue;
      }
      lastProcessedParams.current = currentParamsValue;
    }
  }, [currentParamsValue]);

  const isInitialized = useRef(false);

  // Initial sync on mount
  useEffect(() => {
    if (isInitialized.current) return;
    // Wait for localStorage to be ready
    if (localStorageState === undefined) return;

    synchSearchParamToLocalStorage(
      searchParams,
      searchParamsKey,
      localStorageState,
      setSearchParams,
      defaultValue,
    );
    isInitialized.current = true;
  }, [
    searchParams,
    searchParamsKey,
    localStorageState,
    setSearchParams,
    defaultValue,
  ]);

  const setter = (update: string[] | ((prev: string[]) => string[])) => {
    let newList: string[];
    if (typeof update === 'function') {
      newList = update(stateRef.current);
    } else {
      newList = update;
    }

    // Optimistic update
    stateRef.current = newList;
    setSyncedState(newList);

    // We anticipate this new state, but we MUST NOT update lastProcessedParams here.
    // We want the useEffect to see the "stale" params as "unchanged" from the previous render
    // so it doesn't revert our optimistic update.

    setSearchParams(
      (prevSearchParams) => {
        const newState = getNewStates(
          newList,
          prevSearchParams,
          searchParamsKey,
          defaultValue,
        );
        // Important: Update localStorage synchronously with URL update request
        if (newState.localStorage === undefined) {
          clearLocalStorage();
        } else {
          setLocalStorageState(newState.localStorage);
        }
        return newState.searchParams;
      },
      { replace: true },
    );
  };

  return [syncedState, setter];
};
export const getInitialValue = (
  searchParams: URLSearchParams,
  searchParamsKey: string,
  localStorage: string[] | undefined,
  defaultValue: string[],
) => {
  const searchParamValues = searchParams.getAll(searchParamsKey);
  if (searchParamValues && searchParamValues.length > 0)
    return searchParamValues;

  if (localStorage && localStorage.length > 0) {
    return localStorage;
  }

  return defaultValue;
};

export const synchSearchParamToLocalStorage = (
  searchParams: URLSearchParams,
  searchParamsKey: string,
  localStorage: string[] | undefined,
  setSearchParams: SetURLSearchParams,
  defaultValue: string[],
) => {
  const searchParamValues = searchParams.getAll(searchParamsKey);
  if (searchParamValues && searchParamValues.length > 0) return;
  if (!localStorage || localStorage.length === 0) return;

  if (_.isEqual(_.sortBy(localStorage), _.sortBy(defaultValue))) return;

  const newSearchParams = new URLSearchParams(searchParams);
  for (const el of localStorage) {
    newSearchParams.append(searchParamsKey, el);
  }
  setSearchParams(newSearchParams);
};

export const getNewStates = (
  newList: string[],
  searchParams: URLSearchParams,
  searchParamsKey: string,
  defaultValue: string[],
) => {
  const newSearchParams = new URLSearchParams(searchParams);
  newSearchParams.delete(searchParamsKey); // Avoid duplicates

  // Clear local storage and search params if using the default state
  if (_.isEqual(_.sortBy(newList), _.sortBy(defaultValue))) {
    return {
      localStorage: undefined,
      searchParams: newSearchParams,
    };
  }

  for (const el of newList) {
    newSearchParams.append(searchParamsKey, el);
  }
  return {
    localStorage: newList,
    searchParams: newSearchParams,
  };
};
