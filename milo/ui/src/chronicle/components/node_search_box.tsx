// Copyright 2026 The LUCI Authors.
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

import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import CloseIcon from '@mui/icons-material/Close';
import { Box, Button, IconButton, Typography } from '@mui/material';
import { useRef } from 'react';

export interface NodeSearchBoxProps {
  searchQuery: string;
  matchedNodeIds: string[];
  safeMatchIndex: number;
  isCurrentMatchSelected?: boolean;
  selectedNodeId: string | undefined;
  onSearchChange: (query: string) => void;
  onClearSearch: () => void;
  onNextMatch: () => void;
  onPrevMatch: () => void;
  onStepToMatch: (targetId: string) => void;
  placeholder?: string;
  width?: string | number;
}

/**
 * Reusable search input and match navigation toolbar ("Match X of Y" with `<` and `>` buttons)
 * that can be used across Chronicle views (GraphView, TreeView, TimelineView).
 */
export function NodeSearchBox({
  searchQuery,
  matchedNodeIds,
  safeMatchIndex,
  isCurrentMatchSelected = true,
  selectedNodeId,
  onSearchChange,
  onClearSearch,
  onNextMatch,
  onPrevMatch,
  onStepToMatch,
  placeholder = 'Search nodes...',
  width = '200px',
}: NodeSearchBoxProps) {
  const searchInputRef = useRef<HTMLInputElement>(null);

  return (
    <>
      <Box
        sx={{
          position: 'relative',
          width,
          marginBottom: '6px',
        }}
      >
        <input
          ref={searchInputRef}
          type="text"
          placeholder={placeholder}
          aria-label={placeholder}
          value={searchQuery}
          onChange={(e) => {
            onSearchChange(e.target.value);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && matchedNodeIds.length > 0) {
              e.preventDefault();
              if (e.shiftKey) {
                onPrevMatch();
              } else if (!selectedNodeId && isCurrentMatchSelected) {
                onStepToMatch(matchedNodeIds[safeMatchIndex]);
              } else {
                onNextMatch();
              }
            }
          }}
          style={{
            padding: '8px 28px 8px 8px',
            width: '100%',
            boxSizing: 'border-box',
          }}
        />
        {searchQuery && (
          <IconButton
            size="small"
            aria-label="Clear search"
            title="Clear search"
            onClick={() => {
              onClearSearch();
              searchInputRef.current?.focus();
            }}
            sx={{
              position: 'absolute',
              right: '4px',
              top: '50%',
              transform: 'translateY(-50%)',
              p: '2px',
              color: 'text.secondary',
              '&:hover': {
                color: 'text.primary',
              },
            }}
          >
            <CloseIcon sx={{ fontSize: '16px' }} />
          </IconButton>
        )}
      </Box>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 0.5,
          width,
          height: '24px',
          mb: 0.5,
        }}
      >
        {searchQuery && (
          <>
            <Typography
              variant="body2"
              color="text.secondary"
              data-testid="search-match-display"
              sx={{
                fontSize: '12px',
                userSelect: 'none',
              }}
            >
              {matchedNodeIds.length > 0
                ? `Match ${isCurrentMatchSelected ? safeMatchIndex + 1 : '?'} of ${matchedNodeIds.length}`
                : 'Match 0 of 0'}
            </Typography>
            <Box sx={{ display: 'flex', gap: 0.5 }}>
              <Button
                variant="outlined"
                size="small"
                aria-label="Previous match"
                title="Previous match"
                disabled={matchedNodeIds.length === 0}
                onClick={onPrevMatch}
                sx={{
                  minWidth: '28px',
                  width: '28px',
                  height: '24px',
                  p: 0,
                }}
              >
                <ChevronLeftIcon fontSize="small" />
              </Button>
              <Button
                variant="outlined"
                size="small"
                aria-label="Next match"
                title="Next match"
                disabled={matchedNodeIds.length === 0}
                onClick={onNextMatch}
                sx={{
                  minWidth: '28px',
                  width: '28px',
                  height: '24px',
                  p: 0,
                }}
              >
                <ChevronRightIcon fontSize="small" />
              </Button>
            </Box>
          </>
        )}
      </Box>
    </>
  );
}
