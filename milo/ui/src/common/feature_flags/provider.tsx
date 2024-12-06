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

import { ReactNode, useCallback, useMemo, useState } from 'react';

import {
  ActiveFlag,
  FeatureFlag,
  FeatureFlagStatus,
  FlagObserver,
  FlagsGetterCtx,
  FlagsSetterCtx,
} from './context';

interface Props {
  children: ReactNode;
}

export function FeatureFlagsProvider({ children }: Props) {
  const [availableFlags, setAvailableFlags] = useState<
    Map<FeatureFlag, ActiveFlag>
  >(new Map());
  const addFlagToAvailableFlags = useCallback((status: FeatureFlagStatus) => {
    setAvailableFlags((prevFlags) => {
      const newFlags = new Map(prevFlags);
      const activeFlag = newFlags.get(status.flag);
      if (activeFlag) {
        const flagObservers = new Set(activeFlag.observers);
        flagObservers.add(status.setOverrideStatus);
        newFlags.set(status.flag, {
          status: activeFlag.status,
          observers: flagObservers,
        });
      } else {
        const flagObservers = new Set<FlagObserver>();
        flagObservers.add(status.setOverrideStatus);
        newFlags.set(status.flag, {
          status,
          observers: flagObservers,
        });
      }
      return newFlags;
    });
  }, []);

  const removeFlagFromAvailableFlags = useCallback(
    (flag: FeatureFlag, observer: FlagObserver) => {
      // We can use === here and it would suffice and the configs would be exactly the same objects,
      // but this just ensures that we defend against shallow comparisons.
      setAvailableFlags((prevFlags) => {
        if (!prevFlags.get(flag)) {
          return prevFlags;
        }
        const newFlags = new Map(prevFlags);
        const activeFlag = newFlags.get(flag);
        if (activeFlag) {
          if (activeFlag.observers.size <= 1) {
            newFlags.delete(flag);
          } else {
            const flagObservers = new Set(activeFlag.observers);
            if (flagObservers.has(observer)) {
              flagObservers.delete(observer);
            }
            newFlags.set(flag, {
              status: activeFlag!.status,
              observers: flagObservers,
            });
          }
        }
        return newFlags;
      });
    },
    [],
  );
  const getFlagStatus = useCallback(
    (flag: FeatureFlag) => {
      return availableFlags.get(flag);
    },
    [availableFlags],
  );
  const setterCtxValue = useMemo(() => {
    return {
      addFlagToAvailableFlags,
      removeFlagFromAvailableFlags,
    };
  }, [addFlagToAvailableFlags, removeFlagFromAvailableFlags]);
  return (
    <FlagsSetterCtx.Provider value={setterCtxValue}>
      <FlagsGetterCtx.Provider
        value={{
          availableFlags,
          getFlagStatus,
        }}
      >
        {children}
      </FlagsGetterCtx.Provider>
    </FlagsSetterCtx.Provider>
  );
}
