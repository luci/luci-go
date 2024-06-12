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

import { scaleLinear } from 'd3';
import { ReactNode, useMemo } from 'react';

import { CELL_WIDTH, LINE_HEIGHT, MIN_ROW_HEIGHT } from './constants';
import { ChangeTableCtx } from './context';

export interface ChangepointTableContextProviderProps {
  readonly criticalCommits: readonly string[];
  readonly criticalVariantKeys: readonly string[];
  readonly testVariantBranchCount: number;
  readonly children: ReactNode;
}

export function ChangepointTableContextProvider({
  criticalCommits,
  criticalVariantKeys,
  testVariantBranchCount,
  children,
}: ChangepointTableContextProviderProps) {
  const ctx = useMemo(() => {
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
      criticalCommits,
      commitMap,
      criticalVariantKeys,
      testVariantBranchCount,
      rowHeight,
      xScale,
      yScale,
    };
  }, [criticalCommits, criticalVariantKeys, testVariantBranchCount]);

  return (
    <ChangeTableCtx.Provider value={ctx}>{children}</ChangeTableCtx.Provider>
  );
}
