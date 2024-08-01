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
import { Dispatch, SetStateAction, createContext, useContext } from 'react';

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
export const ConfigCtx = createContext<TimelineConfig | null>(null);

interface RulerStateSetters {
  readonly setDisplay: Dispatch<SetStateAction<boolean>>;
  readonly setX: Dispatch<SetStateAction<number>>;
}

export const RulerStateSettersCtx = createContext<RulerStateSetters | null>(
  null,
);
export const RulerStateCtx = createContext<number | null>(null);

export function useTimelineConfig() {
  const ctx = useContext(ConfigCtx);
  if (ctx === null) {
    throw new Error('useTimelineConfig can only be used in a Timeline');
  }

  return ctx;
}

export function useRulerStateSetters() {
  const ctx = useContext(RulerStateSettersCtx);
  if (ctx === null) {
    throw new Error('useRulerStateSetters can only be used in a Timeline');
  }

  return ctx;
}

export function useRulerState() {
  return useContext(RulerStateCtx);
}
