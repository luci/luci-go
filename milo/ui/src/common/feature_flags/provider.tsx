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

import { ReactNode, useCallback, useState } from 'react';

import { FeatureFlagConfig, FeatureFlagStatus, FlagsCtx } from './context';

interface Props {
  children: ReactNode;
}

export function FeatureFlagsProvider({ children }: Props) {
  const [availableFlags, setAvailableFlags] = useState<
    Map<string, FeatureFlagStatus>
  >(new Map());
  const addFlagToAvailableFlags = useCallback((status: FeatureFlagStatus) => {
    setAvailableFlags((prevFlags) => {
      const newFlags = new Map(prevFlags);
      newFlags.set(`${status.config.namespace}:${status.config.name}`, status);
      return newFlags;
    });
  }, []);

  const removeFlagFromAvailableFlags = useCallback(
    (config: FeatureFlagConfig) => {
      // We can use === here and it would suffice and the configs would be exactly the same objects,
      // but this just ensures that we defend against shallow comparisons.
      setAvailableFlags((prevFlags) => {
        const newFlags = new Map(prevFlags);
        newFlags.delete(`${config.namespace}:${config.name}`);
        return newFlags;
      });
    },
    [],
  );

  return (
    <FlagsCtx.Provider
      value={{
        availableFlags,
        addFlagToAvailableFlags,
        removeFlagFromAvailableFlags,
      }}
    >
      {children}
    </FlagsCtx.Provider>
  );
}
