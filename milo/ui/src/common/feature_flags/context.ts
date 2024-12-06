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

import {
  createContext,
  Dispatch,
  SetStateAction,
  useContext,
  useEffect,
  useMemo,
} from 'react';
import { useLocalStorage } from 'react-use';

import { useAuthState } from '@/common/components/auth_state_provider';

import { hashStringToNum } from '../../generic_libs/tools/string_utils';
import { ANONYMOUS_IDENTITY } from '../api/auth_state';
import { logging } from '../tools/logging';

export interface FeatureFlagConfig {
  /**
   * The namespace that the flag belongs to, used in calculating flag status.
   * For example, `bisection` can be a namespace for a set of flags.
   */
  readonly namespace: string;

  /**
   * The flag name which identify the flag in the namespace.
   * For example, `newColors`.
   */
  readonly name: string;

  /**
   * The rollout percentage, this is the threshold which will be used
   * to check whether the flag is on or not.
   * Note that a rollout of more than 80% is considered fully rolled out,
   * In other words, all users will have the feature turned on if the rollout
   * percentage reaches 80%.
   * This is to encourage teams to clean up their flags early as
   * 80% should give developers good signal of the status of their
   * flag.
   */
  readonly percentage: number;

  /**
   * What is this flag doing, this is displayed to users so that they
   * can control which flags to turn on and off.
   */
  readonly description: string;

  /**
   * The bug tracking this flags rollout.
   */
  readonly trackingBug: string;
}

// DO NOT export this symbol as it is used to seal
// the FeatureFlag interface creation to the `createFeatureFlag` function.
const ConfigSymbol = Symbol('flags_config');

export interface FeatureFlag {
  readonly config: FeatureFlagConfig;
  readonly [ConfigSymbol]: boolean;
}

/**
 * Creates a feature flag holder to be used with `useFeatureFlag` hook.
 *
 * You should call this function a single time for any particular flag,
 * then reuse the instance.
 *
 * Calling this function multiple times with the same namespace and name
 * will cause unexpected results and different parts of the code will
 * calculate different status for the flag.
 */
export function createFeatureFlag(config: FeatureFlagConfig): FeatureFlag {
  return {
    config,
    [ConfigSymbol]: true,
  };
}

/**
 * An observer to the flag changes.
 * This is the setter for the flag in localStorage returned by useLocalStorage.
 */
export type FlagObserver = Dispatch<SetStateAction<string | undefined>>;

/**
 * Wrapper of the feature flag details and it's current status.
 */
export interface FeatureFlagStatus {
  flag: FeatureFlag;
  activeStatus: boolean;
  setOverrideStatus: FlagObserver;
}

/**
 * An active flag is a flag available in a page.
 */
export interface ActiveFlag {
  status: FeatureFlagStatus;
  observers: Set<FlagObserver>;
}

interface FeatureFlagsSetterContext {
  readonly addFlagToAvailableFlags: (flagStatus: FeatureFlagStatus) => void;
  readonly removeFlagFromAvailableFlags: (
    flag: FeatureFlag,
    observer: FlagObserver,
  ) => void;
}

interface FeatureFlagsGetterContext {
  readonly availableFlags: Map<FeatureFlag, ActiveFlag>;
  readonly getFlagStatus: (flag: FeatureFlag) => ActiveFlag | undefined;
}

export const FlagsSetterCtx = createContext<FeatureFlagsSetterContext | null>(
  null,
);
export const FlagsGetterCtx = createContext<FeatureFlagsGetterContext | null>(
  null,
);

export function useAvailableFlags() {
  const ctx = useContext(FlagsGetterCtx);
  if (!ctx) {
    throw new Error(
      'useAvailableFlags can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.availableFlags;
}

export function useGetFlagStatus() {
  const ctx = useContext(FlagsGetterCtx);
  if (!ctx) {
    throw new Error(
      'useGetFlagStatus can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.getFlagStatus;
}

export function useAddFlagToAvailableFlags() {
  const ctx = useContext(FlagsSetterCtx);
  if (!ctx) {
    throw new Error(
      'useAddFlagToAvailableFlags can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.addFlagToAvailableFlags;
}

export function useRemoveFlagFromAvailableFlags() {
  const ctx = useContext(FlagsSetterCtx);
  if (!ctx) {
    throw new Error(
      'useRemoveFlagFromAvailableFlags can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.removeFlagFromAvailableFlags;
}

/**
 * Accepts a feature flag config and returns a boolean of whether the flag is on or off.
 *
 * Important: Using the same namespace and key with different data will cause the first
 * usage of the hook to take precedence over all other declarations.
 *
 * To avoid any unexpected behaviour, please declare the flag config in a common file
 * and reuse the same instance and values.
 *
 * Using this flag in a large list of items can cause performance problems
 * if each item resolves the flag separately.
 * A better use would be to fetch the flag value in a parent
 * component then pass it down to all children in the list.
 */
export function useFeatureFlag(featureFlag: FeatureFlag): boolean {
  const featureFlagConfig = featureFlag.config;
  const { identity } = useAuthState();
  const addFlagToAvailableFlags = useAddFlagToAvailableFlags();
  const removeFlagFromAvailableFlags = useRemoveFlagFromAvailableFlags();

  // Check the local storage to see if the user has overriden the feature flag value.
  const [overrideValue, flagObserver] = useLocalStorage(
    `featureFlag:${featureFlagConfig.namespace}:${featureFlagConfig.name}`,
    '',
    { raw: true },
  );
  const flagStatus = useMemo(() => {
    if (overrideValue) {
      // Values other than on or off are ignored.
      if (overrideValue === 'on') {
        return true;
      } else if (overrideValue === 'off') {
        return false;
      }
    }
    const flagHash = hashStringToNum(
      `${featureFlagConfig.namespace}:${featureFlagConfig.name}:${identity}`,
    );
    // We are using Math.abs as the values returned by hashStringToNum can be negative.
    // We max out at 80% as more than that should be considered fully rolled out and
    // the flag should be removed.
    const userActivationThreshold = Math.abs(flagHash % 100) + 1;

    if (featureFlagConfig.percentage >= 80) {
      logging.warn(
        `Flag ${featureFlagConfig.namespace}:${featureFlagConfig.name} ` +
          `is rolled out to ${featureFlagConfig.percentage}, any percentage over 80 ` +
          `will be capped at 80, if you need to rollout to more than 80% of users, then ` +
          `consider removing the flag as most users will now have it active ` +
          `and you should have a good signal.`,
      );
    }
    return (
      Math.min(userActivationThreshold, 80) <= featureFlagConfig.percentage
    );
  }, [
    overrideValue,
    featureFlagConfig.namespace,
    featureFlagConfig.name,
    featureFlagConfig.percentage,
    identity,
  ]);

  useEffect(() => {
    addFlagToAvailableFlags({
      flag: featureFlag,
      activeStatus: flagStatus,
      setOverrideStatus: flagObserver,
    });
    return () => removeFlagFromAvailableFlags(featureFlag, flagObserver);
  }, [
    addFlagToAvailableFlags,
    flagStatus,
    removeFlagFromAvailableFlags,
    flagObserver,
    featureFlag,
  ]);

  // we always return false if the user is not logged in.
  if (identity === ANONYMOUS_IDENTITY) {
    return false;
  }
  return flagStatus;
}
