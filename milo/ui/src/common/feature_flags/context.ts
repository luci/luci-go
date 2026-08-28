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
  useCallback,
  useContext,
  useEffect,
  useMemo,
} from 'react';
import { useLocalStorage } from 'react-use';

import { useAuthState } from '@/common/components/auth_state_provider';

import { hashStringToNum } from '../../generic_libs/tools/string_utils';
import { ANONYMOUS_IDENTITY } from '../api/auth_state';
import { logging } from '../tools/logging';

export type FeatureEnvironment = 'dev' | 'prod';

export type RolloutPercentage =
  | number
  | { readonly [env in FeatureEnvironment]?: number };

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
   * The rollout percentage threshold for enabling the feature flag.
   * Can be a single number (applies to all allowed environments) or
   * an environment-specific mapping e.g. { dev: 100, prod: 0 }.
   * Note that a rollout of more than 80% is considered fully rolled out.
   */
  readonly percentage: RolloutPercentage;

  /**
   * What is this flag doing, this is displayed to users so that they
   * can control which flags to turn on and off.
   */
  readonly description: string;

  /**
   * The bug tracking this flags rollout.
   */
  readonly trackingBug?: string;

  /**
   * Target environments where this feature flag can be toggled/available.
   * If omitted, defaults to `['dev', 'prod']` (available in all environments).
   */
  readonly allowedEnvironments?: readonly FeatureEnvironment[];
}

export function getCurrentEnvironment(): FeatureEnvironment {
  if (typeof window !== 'undefined') {
    const hostname = window.location.hostname;
    if (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname === '0.0.0.0' ||
      hostname === '::1' ||
      hostname.endsWith('.localhost') ||
      hostname.includes('-dev')
    ) {
      return 'dev';
    }
  }
  return 'prod';
}

export function getFlagRolloutPercentage(
  config: FeatureFlagConfig,
  env: FeatureEnvironment = getCurrentEnvironment(),
): number {
  if (typeof config.percentage === 'number') {
    return config.percentage;
  }
  return config.percentage[env] ?? 0;
}

export function getFeatureFlagKey(
  flagOrConfig: FeatureFlag | FeatureFlagConfig,
): string {
  const config = 'config' in flagOrConfig ? flagOrConfig.config : flagOrConfig;
  return `${config.namespace}:${config.name}`;
}

export function getFeatureFlagLocalStorageKey(
  flagOrConfig: FeatureFlag | FeatureFlagConfig,
): string {
  return `featureFlag:${getFeatureFlagKey(flagOrConfig)}`;
}

export function isFlagAvailableInEnvironment(
  flag: FeatureFlag,
  env: FeatureEnvironment = getCurrentEnvironment(),
): boolean {
  const allowedEnvs = flag.config.allowedEnvironments ?? ['dev', 'prod'];
  return allowedEnvs.includes(env);
}

// DO NOT export this symbol as it is used to seal
// the FeatureFlag interface creation to the `createFeatureFlag` function.
const ConfigSymbol = Symbol('flags_config');

export interface FeatureFlag {
  readonly config: FeatureFlagConfig;
  readonly [ConfigSymbol]: boolean;
}

/**
 * Global registry of all feature flags created via createFeatureFlag.
 * Map is keyed by `${namespace}:${name}` to deduplicate flag registrations during Vite HMR.
 */
export const REGISTERED_FLAGS = new Map<string, FeatureFlag>();

/**
 * Resets the global registry. For testing purposes only.
 */
export function resetRegisteredFlagsForTesting() {
  REGISTERED_FLAGS.clear();
}

/**
 * Creates a feature flag holder to be used with `useFeatureFlag` hook.
 * Automatically pre-registers the flag in `REGISTERED_FLAGS` for global discovery.
 */
export function createFeatureFlag(config: FeatureFlagConfig): FeatureFlag {
  const flag: FeatureFlag = {
    config,
    [ConfigSymbol]: true,
  };
  REGISTERED_FLAGS.set(getFeatureFlagKey(config), flag);
  return flag;
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
 */
export function useFeatureFlag(featureFlag: FeatureFlag): boolean {
  const featureFlagConfig = featureFlag.config;
  const { identity } = useAuthState();
  const addFlagToAvailableFlags = useAddFlagToAvailableFlags();
  const removeFlagFromAvailableFlags = useRemoveFlagFromAvailableFlags();

  const currentEnv = getCurrentEnvironment();
  const isAvailableInEnv = isFlagAvailableInEnvironment(
    featureFlag,
    currentEnv,
  );
  const rolloutPercentage = getFlagRolloutPercentage(
    featureFlagConfig,
    currentEnv,
  );

  // Check local storage for feature flag overrides.
  const [overrideValue, flagObserver] = useLocalStorage(
    getFeatureFlagLocalStorageKey(featureFlagConfig),
    '',
    { raw: true },
  );
  const flagStatus = useMemo(() => {
    if (!isAvailableInEnv) {
      return false;
    }
    if (overrideValue) {
      if (overrideValue === 'on') {
        return true;
      } else if (overrideValue === 'off') {
        return false;
      }
    }
    if (rolloutPercentage <= 0) {
      return false;
    }
    if (rolloutPercentage >= 100) {
      return true;
    }
    if (identity === ANONYMOUS_IDENTITY) {
      return false;
    }
    const flagHash = hashStringToNum(
      `${getFeatureFlagKey(featureFlagConfig)}:${identity}`,
    );
    const userActivationThreshold = Math.abs(flagHash % 100) + 1;

    if (
      currentEnv === 'prod' &&
      rolloutPercentage >= 80 &&
      !(
        featureFlagConfig.allowedEnvironments?.length === 1 &&
        featureFlagConfig.allowedEnvironments[0] === 'dev'
      )
    ) {
      logging.warn(
        `Flag ${getFeatureFlagKey(featureFlagConfig)} ` +
          `is rolled out to ${rolloutPercentage}, any percentage over 80 ` +
          `will be capped at 80, if you need to rollout to more than 80% of users, then ` +
          `consider removing the flag as most users will now have it active ` +
          `and you should have a good signal.`,
      );
    }
    return Math.min(userActivationThreshold, 80) <= rolloutPercentage;
  }, [
    isAvailableInEnv,
    overrideValue,
    featureFlagConfig,
    rolloutPercentage,
    currentEnv,
    identity,
  ]);

  useEffect(() => {
    if (!isAvailableInEnv) {
      return;
    }
    addFlagToAvailableFlags({
      flag: featureFlag,
      activeStatus: flagStatus,
      setOverrideStatus: flagObserver,
    });
    return () => removeFlagFromAvailableFlags(featureFlag, flagObserver);
  }, [
    isAvailableInEnv,
    addFlagToAvailableFlags,
    flagStatus,
    removeFlagFromAvailableFlags,
    flagObserver,
    featureFlag,
  ]);

  return flagStatus;
}

/**
 * Evaluates the value of a feature flag synchronously without requiring a React hook.
 * Respects environment eligibility, localStorage overrides, and percentage rollouts.
 */
export function getFeatureFlagValue(
  featureFlag: FeatureFlag,
  identity: string = ANONYMOUS_IDENTITY,
  env: FeatureEnvironment = getCurrentEnvironment(),
): boolean {
  if (!isFlagAvailableInEnvironment(featureFlag, env)) {
    return false;
  }

  if (typeof window !== 'undefined') {
    try {
      const overrideValue = window.localStorage.getItem(
        getFeatureFlagLocalStorageKey(featureFlag),
      );
      if (overrideValue === 'on') {
        return true;
      } else if (overrideValue === 'off') {
        return false;
      }
    } catch {
      // Ignore localStorage access errors (e.g. strict security contexts)
    }
  }

  const percentage = getFlagRolloutPercentage(featureFlag.config, env);

  if (percentage <= 0) {
    return false;
  }
  if (percentage >= 100) {
    return true;
  }

  if (identity === ANONYMOUS_IDENTITY) {
    return false;
  }

  const flagHash = hashStringToNum(
    `${getFeatureFlagKey(featureFlag)}:${identity}`,
  );
  const userActivationThreshold = Math.abs(flagHash % 100) + 1;
  return Math.min(userActivationThreshold, 80) <= percentage;
}

export function useSetFeatureFlag(flag: FeatureFlag) {
  const availableFlags = useAvailableFlags();

  return useCallback(
    (value: boolean) => {
      const activeFlag = availableFlags.get(flag);
      activeFlag?.observers.forEach((observer) =>
        observer(value ? 'on' : 'off'),
      );
    },
    [availableFlags, flag],
  );
}

export function getEnabledFeatureFlags(): string[] {
  const enabled: string[] = [];
  for (const flag of REGISTERED_FLAGS.values()) {
    if (getFeatureFlagValue(flag)) {
      enabled.push(getFeatureFlagKey(flag));
    }
  }
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key?.startsWith('featureFlag:')) {
          const val = localStorage.getItem(key);
          const flagKey = key.substring('featureFlag:'.length);
          if (val === 'on' && !enabled.includes(flagKey)) {
            enabled.push(flagKey);
          }
        }
      }
    } catch (e) {
      logging.warn(
        'Failed to read feature flag overrides from localStorage',
        e,
      );
    }
  }
  return enabled;
}
