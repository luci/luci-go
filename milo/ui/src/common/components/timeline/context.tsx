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

import { ScaleLinear, ScaleTime, TimeInterval } from 'd3';
import {
  Dispatch,
  ReactNode,
  SetStateAction,
  createContext,
  useContext,
  useMemo,
  useState,
} from 'react';

export interface TimelineConfig {
  readonly startTimeMs: number;
  readonly itemCount: number;
  readonly itemHeight: number;
  readonly sidePanelWidth: number;
  readonly bodyWidth: number;
  readonly xScale: ScaleTime<number, number, never>;
  readonly yScale: ScaleLinear<number, number, never>;
  readonly timeInterval: TimeInterval;
}
const ConfigCtx = createContext<TimelineConfig | null>(null);

interface RulerStateSetters {
  readonly setDisplay: Dispatch<SetStateAction<boolean>>;
  readonly setX: Dispatch<SetStateAction<number>>;
}

const RulerStateSettersCtx = createContext<RulerStateSetters | null>(null);
const RulerStateCtx = createContext<number | null>(null);

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

export function useTimelineConfig() {
  const ctx = useContext(ConfigCtx);
  if (ctx === null) {
    throw new Error('useTimelineConfig can only be used within Timeline');
  }

  return ctx;
}

export function useRulerStateSetters() {
  const ctx = useContext(RulerStateSettersCtx);
  if (ctx === null) {
    throw new Error('useRulerStateSetters can only be used within Timeline');
  }

  return ctx;
}

export function useRulerState() {
  return useContext(RulerStateCtx);
}
