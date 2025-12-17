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

import { Box, styled } from '@mui/material';
import { yellow } from '@mui/material/colors';
import { memo } from 'react';

export interface Highlight {
  start: number;
  length: number;
  active?: boolean;
}

interface TextLineProps {
  line: string;
  emphasized?: boolean;
  highlights?: Highlight[];
}

const HighlightSpan = styled('span', {
  shouldForwardProp: (prop) => prop !== 'active',
})<{ active?: boolean }>(({ theme, active }) => ({
  backgroundColor: active ? theme.palette.warning.main : yellow[300],
  color: active ? theme.palette.warning.contrastText : 'inherit',
}));

export const TextLine = memo(function TextLine({
  line,
  emphasized,
  highlights,
}: TextLineProps) {
  let content: React.ReactNode = line;

  if (highlights && highlights.length > 0) {
    const parts: React.ReactNode[] = [];
    let lastIndex = 0;
    // Sort highlights by start index just in case
    const sortedHighlights = [...highlights].sort((a, b) => a.start - b.start);

    for (const highlight of sortedHighlights) {
      if (highlight.start > lastIndex) {
        parts.push(line.substring(lastIndex, highlight.start));
      }
      parts.push(
        <HighlightSpan key={highlight.start} active={highlight.active}>
          {line.substring(highlight.start, highlight.start + highlight.length)}
        </HighlightSpan>,
      );
      lastIndex = highlight.start + highlight.length;
    }
    if (lastIndex < line.length) {
      parts.push(line.substring(lastIndex));
    }
    content = parts;
  }

  return (
    <Box
      component="pre"
      sx={{
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
        p: 0,
        pl: 1, // Add a small padding to separate from the gutter or start helper
        pr: 2,
        m: 0,
        fontSize: '0.8rem',
        lineHeight: '1.15rem',
        color: emphasized ? 'error.main' : 'text.primary',
        fontFamily: 'monospace',
      }}
    >
      {content}
      {/* Ensure empty lines have height */}
      {line === '' && '\n'}
    </Box>
  );
});
