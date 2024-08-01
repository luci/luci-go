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

import { ReactNode, useMemo, useState } from 'react';

import {
  ConfigCtx,
  RulerStateCtx,
  RulerStateSettersCtx,
  TimelineConfig,
} from './context';

export interface TimelineContextProviderProps {
  readonly config: TimelineConfig;
  readonly children: ReactNode;
}

export function TimelineContextProvider({
  config,
  children,
}: TimelineContextProviderProps) {
  const [rulerX, setRulerX] = useState(0);
  const [displayRuler, setDisplayRuler] = useState(false);
  const rulerStateSetters = useMemo(
    () => ({ setX: setRulerX, setDisplay: setDisplayRuler }),
    [setRulerX, setDisplayRuler],
  );

  return (
    <ConfigCtx.Provider value={config}>
      <RulerStateSettersCtx.Provider value={rulerStateSetters}>
        <RulerStateCtx.Provider value={displayRuler ? rulerX : null}>
          {children}
        </RulerStateCtx.Provider>
      </RulerStateSettersCtx.Provider>
    </ConfigCtx.Provider>
  );
}
