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

import { createContext, ReactNode, useContext, useMemo } from 'react';

import { OutputSegment, OutputTestVariantBranch } from '@/analysis/types';

interface BlamelistContext {
  readonly segmentsSortedByEnd: readonly OutputSegment[];
  readonly segmentsSortedByStartUpperBound: readonly OutputSegment[];
}

const Ctx = createContext<BlamelistContext | undefined>(undefined);

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
      (seg1, seg2) => parseInt(seg1.endPosition) - parseInt(seg2.endPosition),
    );
    const segmentsSortedByStartUpperBound = testVariantBranch.segments
      .filter((seg) => seg.hasStartChangepoint)
      .sort(
        (seg1, seg2) =>
          parseInt(seg1.startPositionUpperBound99th) -
          parseInt(seg2.startPositionUpperBound99th),
      );

    return {
      segmentsSortedByEnd,
      segmentsSortedByStartUpperBound,
    };
  }, [testVariantBranch]);

  return <Ctx.Provider value={ctx}>{children}</Ctx.Provider>;
}

/**
 * Find the segment where the commit position is in range
 * [segment.endPosition, segment.startPosition].
 */
export function useSegmentWithCommit(commitPosition: string) {
  const ctx = useContext(Ctx);
  if (ctx === undefined) {
    throw new Error(
      'useSegmentWithCommit can only be used within BlamelistTable',
    );
  }

  const matchedSegment = useMemo(() => {
    const cp = parseInt(commitPosition);

    // Find a segment containing the commit position.
    // Linear search should be good enough since the array should be very small.
    for (const seg of ctx.segmentsSortedByEnd) {
      const end = parseInt(seg.endPosition);

      if (end < cp) {
        continue;
      }
      const start = parseInt(seg.startPosition);
      if (start > cp) {
        return null;
      }
      return seg;
    }
    return null;
  }, [ctx, commitPosition]);

  return matchedSegment;
}

/**
 * Find the segments where the commit position is in range
 * [segment.startPositionUpperBound99th, segment.startPositionLowerBound99th].
 */
export function useChangepointsWithCommit(
  commitPosition: string,
): readonly OutputSegment[] {
  const ctx = useContext(Ctx);
  if (ctx === undefined) {
    throw new Error(
      'useChangepointsWithCommit can only be used within BlamelistTable',
    );
  }

  const matchedSegments = useMemo(() => {
    const cp = parseInt(commitPosition);
    const segments: OutputSegment[] = [];

    // Find a segment containing the commit position.
    // Linear search should be good enough since the array should be very small.
    for (const seg of ctx.segmentsSortedByStartUpperBound) {
      const upper = parseInt(seg.startPositionUpperBound99th);

      if (upper < cp) {
        continue;
      }
      const lower = parseInt(seg.startPositionLowerBound99th);
      if (lower > cp) {
        break;
      }
      segments.push(seg);
    }
    return segments;
  }, [ctx, commitPosition]);

  return matchedSegments;
}
