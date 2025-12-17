// Copyright 2025 The LUCI Authors.
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

import { useState, useMemo, useEffect, ChangeEvent } from 'react';

import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { Highlight } from './text_line';

export interface Match {
  lineIndex: number;
  start: number;
  length: number;
}

interface LineItem {
  type: 'line';
  line: string;
  emphasized?: boolean;
  highlights?: Highlight[];
  originalLineIndex?: number;
}

interface DividerItem {
  type: 'divider';
  numLines?: number;
  onExpandStart?: (n: number) => void;
  onExpandEnd?: (n: number) => void;
}

interface LoadingItem {
  type: 'loading';
}

export type Item = LineItem | DividerItem | LoadingItem;

interface ExpandedState {
  start: number;
  end: number;
}

export function useArtifactSearch(lines: string[]) {
  const [searchQuery, setSearchQuery] = useState('');
  const [currentMatchIndex, setCurrentMatchIndex] = useState(-1);

  // Calculate matches
  const matches = useMemo(() => {
    if (!searchQuery) return [];
    const res: Match[] = [];
    const lowerQuery = searchQuery.toLowerCase();
    // Simple substring search
    lines.forEach((line, lineIdx) => {
      let startIndex = 0;
      const lowerLine = line.toLowerCase();
      while ((startIndex = lowerLine.indexOf(lowerQuery, startIndex)) > -1) {
        res.push({
          lineIndex: lineIdx,
          start: startIndex,
          length: lowerQuery.length,
        });
        startIndex += lowerQuery.length;
      }
    });
    return res;
  }, [lines, searchQuery]);

  // Reset match index when matches change
  useEffect(() => {
    setCurrentMatchIndex(matches.length > 0 ? 0 : -1);
  }, [matches.length]);

  const activeMatch =
    currentMatchIndex >= 0 && currentMatchIndex < matches.length
      ? matches[currentMatchIndex]
      : null;

  // Map lineIndex -> Highlights[]
  const lineMatches = useMemo(() => {
    const map = new Map<number, Highlight[]>();
    if (!matches.length) {
      return map;
    }

    matches.forEach((m, idx) => {
      const isActive = idx === currentMatchIndex;
      const h: Highlight = {
        start: m.start,
        length: m.length,
        active: isActive,
      };
      const existing = map.get(m.lineIndex) || [];
      existing.push(h);
      map.set(m.lineIndex, existing);
    });
    return map;
  }, [matches, currentMatchIndex]);

  const handleNextMatch = () => {
    if (matches.length === 0) return;
    setCurrentMatchIndex((prev) => (prev + 1) % matches.length);
  };

  const handlePrevMatch = () => {
    if (matches.length === 0) return;
    setCurrentMatchIndex(
      (prev) => (prev - 1 + matches.length) % matches.length,
    );
  };

  const handleSearchChange = (e: ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(e.target.value);
  };

  return {
    searchQuery,
    setSearchQuery,
    currentMatchIndex,
    matches,
    activeMatch,
    lineMatches,
    handleNextMatch,
    handlePrevMatch,
    handleSearchChange,
  };
}

export function useArtifactViewItems(
  lines: string[],
  failureOnlyRanges:
    | readonly CompareArtifactLinesResponse_FailureOnlyRange[]
    | undefined,
  isFullLoading: boolean | undefined,
  lineMatches: Map<number, Highlight[]>,
) {
  // key: segment index
  const [expandedRegions, setExpandedRegions] = useState<
    Record<number, ExpandedState>
  >({});

  const items = useMemo(() => {
    // If no ranges provided, just show all lines (Simple View)
    if (!failureOnlyRanges) {
      const allItems: Item[] = lines.map((line, idx) => ({
        type: 'line',
        line,
        originalLineIndex: idx,
        highlights: lineMatches.get(idx),
      }));
      if (isFullLoading) {
        allItems.push({ type: 'loading' });
      }
      return allItems;
    }

    const newItems: Item[] = [];
    let lastLineRendered = 0;
    // We strictly use index in this loop as the key for expansion state
    let segmentIndex = 0;

    for (const range of failureOnlyRanges) {
      // Stop if we've reached the end of available content
      if (lastLineRendered >= lines.length) {
        break;
      }

      // Hidden Segment (Gap)
      if (range.startLine > lastLineRendered) {
        const segStart = lastLineRendered;
        const segEnd = Math.min(range.startLine, lines.length);
        const segmentLen = segEnd - segStart;

        if (segmentLen > 0) {
          const expanded = expandedRegions[segmentIndex] || {
            start: 0,
            end: 0,
          };

          // Ensure we don't expand more than available
          const startCount = Math.min(expanded.start, segmentLen);
          const endCount = Math.min(expanded.end, segmentLen - startCount);
          const hiddenCount = segmentLen - startCount - endCount;

          // Add Start Lines
          for (let i = 0; i < startCount; i++) {
            const lineIdx = segStart + i;
            newItems.push({
              type: 'line',
              line: lines[lineIdx],
              originalLineIndex: lineIdx,
              highlights: lineMatches.get(lineIdx),
            });
          }

          // Add Divider
          if (hiddenCount > 0) {
            const currentSegIndex = segmentIndex;
            newItems.push({
              type: 'divider',
              numLines: hiddenCount,
              onExpandStart:
                segmentIndex > 0 // Not first segment -> Can expand start (Top)
                  ? (n) =>
                      setExpandedRegions((prev) => {
                        const current = prev[currentSegIndex] || {
                          start: 0,
                          end: 0,
                        };
                        return {
                          ...prev,
                          [currentSegIndex]: {
                            ...current,
                            start: current.start + n,
                          },
                        };
                      })
                  : undefined,
              onExpandEnd: (n) =>
                setExpandedRegions((prev) => {
                  const current = prev[currentSegIndex] || {
                    start: 0,
                    end: 0,
                  };
                  return {
                    ...prev,
                    [currentSegIndex]: {
                      ...current,
                      end: current.end + n,
                    },
                  };
                }),
            });
          }

          // Add End Lines
          for (let i = 0; i < endCount; i++) {
            const lineIdx = segEnd - endCount + i;
            newItems.push({
              type: 'line',
              line: lines[lineIdx],
              originalLineIndex: lineIdx,
              highlights: lineMatches.get(lineIdx),
            });
          }
        }
        segmentIndex++;
      }

      // If the failure range starts beyond our content, we stop.
      if (range.startLine >= lines.length) {
        break;
      }

      // Visible Segment (Failure)
      const rangeEnd = Math.min(range.endLine, lines.length);
      for (let i = range.startLine; i < rangeEnd; i++) {
        newItems.push({
          type: 'line',
          line: lines[i],
          emphasized: true,
          originalLineIndex: i,
          highlights: lineMatches.get(i),
        });
      }
      lastLineRendered = rangeEnd;
    }

    // Final Hidden Segment
    if (lastLineRendered < lines.length) {
      const segStart = lastLineRendered;
      const segEnd = lines.length;
      const segmentLen = segEnd - segStart;

      const expanded = expandedRegions[segmentIndex] || { start: 0, end: 0 };
      const startCount = Math.min(expanded.start, segmentLen);
      const endCount = Math.min(expanded.end, segmentLen - startCount);
      const hiddenCount = segmentLen - startCount - endCount;

      // Add Start Lines
      for (let i = 0; i < startCount; i++) {
        const lineIdx = segStart + i;
        newItems.push({
          type: 'line',
          line: lines[lineIdx],
          originalLineIndex: lineIdx,
          highlights: lineMatches.get(lineIdx),
        });
      }

      if (hiddenCount > 0) {
        const currentSegIndex = segmentIndex;
        newItems.push({
          type: 'divider',
          numLines: hiddenCount,

          onExpandStart: (n) =>
            setExpandedRegions((prev) => {
              const current = prev[currentSegIndex] || {
                start: 0,
                end: 0,
              };
              return {
                ...prev,
                [currentSegIndex]: {
                  ...current,
                  start: current.start + n,
                },
              };
            }),
          onExpandEnd: undefined,
        });
      }

      // Add End Lines
      for (let i = 0; i < endCount; i++) {
        const lineIdx = segEnd - endCount + i;
        newItems.push({
          type: 'line',
          line: lines[lineIdx],
          originalLineIndex: lineIdx,
          highlights: lineMatches.get(lineIdx),
        });
      }
    }

    if (isFullLoading) {
      newItems.push({ type: 'loading' });
    }

    return newItems;
  }, [lines, failureOnlyRanges, isFullLoading, expandedRegions, lineMatches]);

  return items;
}
