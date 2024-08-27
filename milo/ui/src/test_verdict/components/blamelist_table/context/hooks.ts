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

import { useContext, useMemo } from 'react';

import { OutputSegment } from '@/analysis/types';

import { BlamelistCtx } from './context';

export function useProject() {
  const ctx = useContext(BlamelistCtx);
  if (ctx === undefined) {
    throw new Error(
      'useProject can only be used in a BlamelistContextProvider',
    );
  }

  return ctx.project;
}

/**
 * Find the segment where the commit position is in range
 * [segment.endPosition, segment.startPosition].
 */
export function useSegmentWithCommit(commitPosition: string) {
  const ctx = useContext(BlamelistCtx);
  if (ctx === undefined) {
    throw new Error(
      'useSegmentWithCommit can only be used in a BlamelistContextProvider',
    );
  }

  const matchedSegment = useMemo(() => {
    const cp = parseInt(commitPosition);

    // Find a segment containing the commit position.
    // Linear search should be good enough since the array should be very small.
    for (const seg of ctx.segmentsSortedByEnd) {
      const start = parseInt(seg.startPosition);
      if (start > cp) {
        continue;
      }
      const end = parseInt(seg.endPosition);
      if (end < cp) {
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
export function useStartPointsWithCommit(
  commitPosition: string,
): ReadonlyArray<readonly [segIndex: number, OutputSegment]> {
  const ctx = useContext(BlamelistCtx);
  if (ctx === undefined) {
    throw new Error(
      'useStartPointsWithCommit can only be used in a BlamelistContextProvider',
    );
  }

  const matchedSegments = useMemo(() => {
    const cp = parseInt(commitPosition);
    const segments: Array<readonly [number, OutputSegment]> = [];

    // Find a segment containing the commit position.
    // Linear search should be good enough since the array should be very small.
    for (const entry of ctx.segmentsSortedByStartUpperBound) {
      const seg = entry[1];
      const lower = parseInt(seg.startPositionLowerBound99th);
      if (lower > cp) {
        continue;
      }
      const upper = parseInt(seg.startPositionUpperBound99th);
      if (upper < cp) {
        break;
      }
      segments.push(entry);
    }
    return segments;
  }, [ctx, commitPosition]);

  return matchedSegments;
}
