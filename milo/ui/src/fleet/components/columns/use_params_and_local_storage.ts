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
): [string[], (new_value: string[]) => void] => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [localStorage, setLocalStorage, clearLocalStorage] =
    useLocalStorage<string[]>(localStorageKey);

  // improves performance
  const justCalledSetter = useRef(false);

  // We also keep an internal state, this allows us to return this to the user
  // so they can speedup their computations and not wait for
  // query parameters and local storage to update
  const [syncedState, setSyncedState] = useState(
    getInitialValue(searchParams, searchParamsKey, localStorage, defaultValue),
  );

  // Sets the query parameters if they are not set and only local storage is present
  // useful on page load
  useEffect(() => {
    synchSearchParamToLocalStorage(
      searchParams,
      searchParamsKey,
      localStorage,
      setSearchParams,
      defaultValue,
    );
  }, [
    defaultValue,
    localStorage,
    searchParams,
    searchParamsKey,
    setSearchParams,
  ]);

  // Reset internal state if underlying locaStorage or searchParams change
  useEffect(() => {
    if (justCalledSetter.current) {
      justCalledSetter.current = false;
      return;
    }

    const newValue = getInitialValue(
      searchParams,
      searchParamsKey,
      localStorage,
      defaultValue,
    );
    if (!_.isEqual(newValue, syncedState)) {
      setSyncedState(newValue);
    }
  }, [
    searchParamsKey,
    localStorageKey,
    searchParams,
    localStorage,
    defaultValue,
    syncedState,
  ]);

  const setter = (newList: string[]) => {
    justCalledSetter.current = true;
    setSyncedState(newList);

    const newState = getNewStates(
      newList,
      searchParams,
      searchParamsKey,
      defaultValue,
    );
    if (newState.localStorage === undefined) clearLocalStorage();
    else setLocalStorage(newState.localStorage);

    setSearchParams(newState.searchParams);
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
