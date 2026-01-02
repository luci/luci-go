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

import { Box, Typography, styled } from '@mui/material';
import { useWindowVirtualizer } from '@tanstack/react-virtual';
import {
  useMemo,
  useState,
  useRef,
  useLayoutEffect,
  useEffect,
  useCallback,
} from 'react';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useInvocation } from '@/test_investigation/context';
import { isAnTSInvocation } from '@/test_investigation/utils/test_info_utils';

import { ArtifactContentHeader } from '../header';
import { ArtifactContentSearchBar } from '../header';
import { ArtifactContentSearchMenu } from '../header';
import { useArtifactViewItems, useArtifactSearch } from '../hooks';

import { Divider } from './divider';
import { TextLine } from './text_line';

const LINE_NUMBER_WIDTH = 50;

const LineGutter = styled(Box)(({ theme }) => ({
  width: `${LINE_NUMBER_WIDTH}px`,
  minWidth: `${LINE_NUMBER_WIDTH}px`,
  textAlign: 'right',
  paddingLeft: theme.spacing(3),
  paddingRight: theme.spacing(2),
  color: theme.palette.text.secondary,
  userSelect: 'none',
  fontSize: '0.8rem',
  lineHeight: '1.15rem',
  fontFamily: 'monospace',
  opacity: 0.7,
}));

interface VirtualizedArtifactContentProps {
  content: string;
  failureOnlyRanges?: readonly CompareArtifactLinesResponse_FailureOnlyRange[];
  isFullLoading?: boolean;
  artifact: Artifact;
  hasPassingResults: boolean;
  isLogComparisonPossible: boolean;
  showLogComparison: boolean;
  onToggleLogComparison: () => void;
  actions?: React.ReactNode;
}

export function VirtualizedArtifactContent({
  content,
  failureOnlyRanges,
  isFullLoading,
  artifact,
  hasPassingResults,
  isLogComparisonPossible,
  showLogComparison,
  onToggleLogComparison,
  actions,
}: VirtualizedArtifactContentProps) {
  const lines = useMemo(() => content.split('\n'), [content]);
  const listRef = useRef<HTMLDivElement>(null);
  const invocation = useInvocation();
  const isAnTS = isAnTSInvocation(invocation);

  const {
    searchQuery,
    matches,
    activeMatch,
    lineMatches,
    currentMatchIndex,
    handleNextMatch,
    handlePrevMatch,
    handleSearchChange,
  } = useArtifactSearch(lines);

  const items = useArtifactViewItems(
    lines,
    failureOnlyRanges,
    isFullLoading,
    lineMatches,
  );

  const [scrollMargin, setScrollMargin] = useState(0);

  useLayoutEffect(() => {
    if (!listRef.current) {
      return;
    }
    // Calculate accurate offset from the top of the document
    const updateOffset = () => {
      if (listRef.current) {
        const rect = listRef.current.getBoundingClientRect();
        // The scroll margin is the absolute top position of the container
        setScrollMargin(window.scrollY + rect.top);
      }
    };

    // Initial measurement
    updateOffset();

    // Re-measure on resize and on every render to catch layout shifts
    window.addEventListener('resize', updateOffset);
    return () => window.removeEventListener('resize', updateOffset);
  }, []);

  const virtualizer = useWindowVirtualizer({
    count: items.length,
    estimateSize: () => 20, // Approximate line height
    overscan: 10,
    scrollMargin,
  });

  const scrollToMatch = useCallback(
    (match: { lineIndex: number }) => {
      const itemIndex = items.findIndex(
        (item) =>
          item.type === 'line' && item.originalLineIndex === match.lineIndex,
      );
      if (itemIndex !== -1) {
        virtualizer.scrollToIndex(itemIndex, { align: 'center' });
      }
    },
    [items, virtualizer],
  );

  useEffect(() => {
    if (activeMatch) {
      scrollToMatch(activeMatch);
    }
  }, [activeMatch, scrollToMatch]);

  return (
    <div ref={listRef} style={{ minWidth: 0 }}>
      <ArtifactContentHeader
        artifact={artifact}
        contentData={{ isText: true, data: content }}
        sticky
        searchBox={
          <ArtifactContentSearchBar
            searchQuery={searchQuery}
            onSearchChange={handleSearchChange}
            currentMatchIndex={currentMatchIndex}
            totalMatches={matches.length}
            onNext={handleNextMatch}
            onPrev={handlePrevMatch}
          />
        }
        actions={
          <ArtifactContentSearchMenu
            artifact={artifact}
            isLogComparisonPossible={isLogComparisonPossible}
            showLogComparison={showLogComparison}
            hasPassingResults={hasPassingResults}
            onToggleLogComparison={onToggleLogComparison}
            isAnTS={isAnTS}
            invocation={invocation as Invocation}
            actions={actions}
          />
        }
      />
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const item = items[virtualItem.index];
          let innerContent;

          if (item.type === 'line') {
            innerContent = (
              <TextLine
                line={item.line || ''}
                emphasized={item.emphasized}
                highlights={item.highlights}
              />
            );
          } else if (item.type === 'divider') {
            innerContent = (
              <Divider
                numLines={item.numLines || 0}
                onExpandStart={item.onExpandStart}
                onExpandEnd={item.onExpandEnd}
              />
            );
          } else if (item.type === 'loading') {
            innerContent = (
              <Box
                sx={{
                  p: 2,
                  display: 'flex',
                  justifyContent: 'center',
                  color: 'text.secondary',
                  fontStyle: 'italic',
                }}
              >
                <Typography>Loading more content...</Typography>
              </Box>
            );
          }

          return (
            <div
              key={virtualItem.key}
              ref={virtualizer.measureElement}
              data-index={virtualItem.index}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${
                  virtualItem.start - virtualizer.options.scrollMargin
                }px)`,
                display: 'flex',
                flexDirection: 'row',
              }}
            >
              <LineGutter>
                {item.type === 'line' && item.originalLineIndex !== undefined
                  ? item.originalLineIndex + 1
                  : ''}
              </LineGutter>
              <Box sx={{ flex: 1, minWidth: 0 }}>{innerContent}</Box>
            </div>
          );
        })}
      </div>
    </div>
  );
}
