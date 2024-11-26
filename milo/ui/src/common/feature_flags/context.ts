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

import { createContext, useContext, useEffect, useMemo } from 'react';
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

export interface FeatureFlagStatus {
  config: FeatureFlagConfig;
  activeStatus: boolean;
}

interface FeatureFlagsContext {
  availableFlags: Map<string, FeatureFlagStatus>;
  addFlagToAvailableFlags: (flagStatus: FeatureFlagStatus) => void;
  removeFlagFromAvailableFlags: (config: FeatureFlagConfig) => void;
}

export const FlagsCtx = createContext<FeatureFlagsContext | null>(null);

export function useAvailableFlags() {
  const ctx = useContext(FlagsCtx);
  if (!ctx) {
    throw new Error(
      'useAvailableFlags can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.availableFlags;
}

export function useAddFlagToAvailableFlags() {
  const ctx = useContext(FlagsCtx);
  if (!ctx) {
    throw new Error(
      'useAddFlagToAvailableFlags can only be used in a FeatureFlagsProvider',
    );
  }
  return ctx.addFlagToAvailableFlags;
}

export function useRemoveFlagFromAvailableFlags() {
  const ctx = useContext(FlagsCtx);
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
export function useFeatureFlag(featureFlagConfig: FeatureFlagConfig): boolean {
  const { identity } = useAuthState();
  const addFlagToAvailableFlags = useAddFlagToAvailableFlags();
  const removeFlagFromAvailableFlags = useRemoveFlagFromAvailableFlags();
  const stableFeatureFlagsConfig: FeatureFlagConfig = useMemo(() => {
    return {
      namespace: featureFlagConfig.namespace,
      name: featureFlagConfig.name,
      description: featureFlagConfig.description,
      percentage: featureFlagConfig.percentage,
      trackingBug: featureFlagConfig.trackingBug,
    };
  }, [
    featureFlagConfig.description,
    featureFlagConfig.namespace,
    featureFlagConfig.name,
    featureFlagConfig.percentage,
    featureFlagConfig.trackingBug,
  ]);

  // Check the local storage to see if the user has overriden the feature flag value.
  const [overrideValue] = useLocalStorage(
    `featureFlag:${stableFeatureFlagsConfig.namespace}:${stableFeatureFlagsConfig.name}`,
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
      `${stableFeatureFlagsConfig.namespace}:${stableFeatureFlagsConfig.name}:${identity}`,
    );
    // We are using Math.abs as the values returned by hashStringToNum can be negative.
    // We max out at 80% as more than that should be considered fully rolled out and
    // the flag should be removed.
    const userActivationThreshold = Math.abs(flagHash % 100) + 1;

    if (stableFeatureFlagsConfig.percentage) {
      logging.warn(
        `Flag ${stableFeatureFlagsConfig.namespace}:${stableFeatureFlagsConfig.name} ` +
          `is rolled out to ${stableFeatureFlagsConfig.percentage}, any percentage over 80 ` +
          `will be capped at 80, if you need to rollout to more than 80% of usrs, then ` +
          `consider removing the flag as most users will now have it active ` +
          `and you should have a good signal.`,
      );
    }
    return (
      Math.min(userActivationThreshold, 80) <=
      stableFeatureFlagsConfig.percentage
    );
  }, [
    identity,
    stableFeatureFlagsConfig.namespace,
    stableFeatureFlagsConfig.name,
    stableFeatureFlagsConfig.percentage,
    overrideValue,
  ]);

  useEffect(() => {
    addFlagToAvailableFlags({
      config: stableFeatureFlagsConfig,
      activeStatus: flagStatus,
    });
    return () => removeFlagFromAvailableFlags(stableFeatureFlagsConfig);
  }, [
    addFlagToAvailableFlags,
    stableFeatureFlagsConfig,
    flagStatus,
    removeFlagFromAvailableFlags,
  ]);

  // we always return false if the user is not logged in.
  if (identity === ANONYMOUS_IDENTITY) {
    return false;
  }
  return flagStatus;
}
