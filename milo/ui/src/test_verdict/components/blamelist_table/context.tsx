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

import { createContext, ReactNode, useMemo } from 'react';

import { OutputSegment, OutputTestVariantBranch } from '@/analysis/types';

interface BlamelistContext {
  readonly project: string;
  /**
   * Segments sorted by their end commit position in descending order.
   */
  readonly segmentsSortedByEnd: readonly OutputSegment[];
  /**
   * Segments sorted by their start upper bound commit position in descending
   * order.
   */
  readonly segmentsSortedByStartUpperBound: ReadonlyArray<
    readonly [segIndex: number, OutputSegment]
  >;
}

export const BlamelistCtx = createContext<BlamelistContext | undefined>(
  undefined,
);

export interface BlamelistContextProviderProps {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly children: ReactNode;
}

export function BlamelistContextProvider({
  testVariantBranch,
  children,
}: BlamelistContextProviderProps) {
  const ctx = useMemo(() => {
    const segmentsSortedByEnd = [...testVariantBranch.segments].sort(
      (seg1, seg2) => parseInt(seg2.endPosition) - parseInt(seg1.endPosition),
    );
    const segmentsSortedByStartUpperBound = [
      ...testVariantBranch.segments.entries(),
    ]
      .filter(([, seg]) => seg.hasStartChangepoint)
      .sort(
        ([, seg1], [, seg2]) =>
          parseInt(seg2.startPositionUpperBound99th) -
          parseInt(seg1.startPositionUpperBound99th),
      );

    return {
      project: testVariantBranch.project,
      segmentsSortedByEnd,
      segmentsSortedByStartUpperBound,
    };
  }, [testVariantBranch]);

  return <BlamelistCtx.Provider value={ctx}>{children}</BlamelistCtx.Provider>;
}
