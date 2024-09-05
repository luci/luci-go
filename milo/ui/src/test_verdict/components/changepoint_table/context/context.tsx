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

import { ScaleLinear, scaleLinear } from 'd3';
import { createContext, Dispatch, ReactNode, useMemo, useReducer } from 'react';

import { CELL_WIDTH, LINE_HEIGHT, MIN_ROW_HEIGHT } from '../constants';
import { Action, BlamelistState, reducer } from '../reducer';

export const BlamelistDispatcherCtx = createContext<
  Dispatch<Action> | undefined
>(undefined);
export const BlamelistStateCtx = createContext<BlamelistState | undefined>(
  undefined,
);

export interface BlamelistStateProviderProps {
  readonly children: ReactNode;
}

export function BlamelistStateProvider({
  children,
}: BlamelistStateProviderProps) {
  const [state, dispatch] = useReducer(reducer, {
    commitPositionRange: null,
    testVariantBranch: null,
    focusCommitPosition: null,
  });

  return (
    <BlamelistDispatcherCtx.Provider value={dispatch}>
      <BlamelistStateCtx.Provider value={state}>
        {children}
      </BlamelistStateCtx.Provider>
    </BlamelistDispatcherCtx.Provider>
  );
}

export const ChangeTableCtx = createContext<ChangepointTableConfig | undefined>(
  undefined,
);

interface ChangepointTableConfig {
  readonly gitilesHost: string;
  readonly gitilesRepository: string;
  readonly gitilesRef: string;
  /**
   * Sorted in descending order.
   */
  readonly criticalCommits: readonly string[];
  readonly commitMap: { [key: string]: number };
  readonly criticalVariantKeys: readonly string[];
  readonly testVariantBranchCount: number;
  readonly rowHeight: number;
  readonly xScale: ScaleLinear<number, number, never>;
  readonly yScale: ScaleLinear<number, number, never>;
}

export interface ChangepointTableContextProviderProps {
  readonly gitilesHost: string;
  readonly gitilesRepository: string;
  readonly gitilesRef: string;
  /**
   * Sorted in descending order.
   */
  readonly criticalCommits: readonly string[];
  readonly criticalVariantKeys: readonly string[];
  readonly testVariantBranchCount: number;
  readonly children: ReactNode;
}

export function ChangepointTableContextProvider({
  gitilesHost,
  gitilesRepository,
  gitilesRef,
  criticalCommits,
  criticalVariantKeys,
  testVariantBranchCount,
  children,
}: ChangepointTableContextProviderProps) {
  const ctx = useMemo<ChangepointTableConfig>(() => {
    const commitMap = Object.fromEntries(criticalCommits.map((c, i) => [c, i]));

    const rowHeight = Math.max(
      (criticalVariantKeys.length + 1) * LINE_HEIGHT,
      MIN_ROW_HEIGHT,
    );
    const yScale = scaleLinear()
      .domain([0, testVariantBranchCount])
      .range([0, testVariantBranchCount * rowHeight]);
    const xScale = scaleLinear()
      .domain([0, criticalCommits.length])
      .range([0, criticalCommits.length * CELL_WIDTH]);

    return {
      gitilesHost,
      gitilesRepository,
      gitilesRef,
      criticalCommits,
      commitMap,
      criticalVariantKeys,
      testVariantBranchCount,
      rowHeight,
      xScale,
      yScale,
    };
  }, [
    gitilesHost,
    gitilesRepository,
    gitilesRef,
    criticalCommits,
    criticalVariantKeys,
    testVariantBranchCount,
  ]);

  return (
    <ChangeTableCtx.Provider value={ctx}>{children}</ChangeTableCtx.Provider>
  );
}
