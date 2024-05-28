// Copyright 2022 The LUCI Authors.
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

import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { Box, SxProps, Theme } from '@mui/material';
import { createContext, useContext } from 'react';

const ExpandedContext = createContext(false);

export interface ExpandableEntryHeaderProps {
  readonly onToggle: (expand: boolean) => void;
  readonly sx?: SxProps<Theme>;
  readonly children: React.ReactNode;
}

/**
 * Renders the header of an <ExpandableEntry />.
 */
export function ExpandableEntryHeader({
  onToggle,
  sx,
  children,
}: ExpandableEntryHeaderProps) {
  const expanded = useContext(ExpandedContext);

  return (
    <Box
      onClick={() => onToggle(!expanded)}
      sx={{
        display: 'grid',
        gridTemplateColumns: '24px 1fr',
        gridGap: '5px',
        cursor: 'pointer',
        lineHeight: '24px',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'center',
          alignContent: 'center',
        }}
      >
        {expanded ? <ExpandMore /> : <ChevronRight />}
      </Box>
      {/* Use a Box isolate the children from grid layout. */}
      <Box sx={{ overflow: 'hidden', whiteSpace: 'nowrap', ...sx }}>
        {children}
      </Box>
    </Box>
  );
}

export interface ExpandableEntryBodyProps {
  /**
   * Configure whether the content ruler should be rendered.
   * * visible: the default option. Renders the content ruler.
   * * invisible: hide the content ruler but keep the indentation.
   * * none: hide the content ruler and don't keep the indentation.
   */
  readonly ruler?: 'visible' | 'invisible' | 'none';
  readonly sx?: SxProps<Theme>;
  readonly children: React.ReactNode;
}

/**
 * Renders the body of an <ExpandableEntry />.
 * The content is hidden when the entry is collapsed.
 */
export function ExpandableEntryBody({
  ruler = 'visible',
  sx,
  children,
}: ExpandableEntryBodyProps) {
  const expanded = useContext(ExpandedContext);

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: ruler === 'none' ? '1fr' : '24px 1fr',
        gridGap: '5px',
      }}
    >
      <Box
        sx={{
          display: ruler === 'none' ? 'none' : '',
          visibility: ruler === 'invisible' ? 'hidden' : '',
          borderLeft: '1px solid var(--divider-color)',
          width: '0px',
          marginLeft: '11.5px',
        }}
      ></Box>
      {/* Use a Box isolate the children from grid layout. */}
      <Box sx={sx}>{expanded ? children : <></>}</Box>
    </Box>
  );
}

export interface ExpandableEntryProps {
  readonly expanded: boolean;
  readonly sx?: SxProps<Theme>;
  /**
   * The first child should be an <ExpandableEntryHeader />.
   * The second child should be an <ExpandableEntryBody />.
   */
  readonly children: [JSX.Element, JSX.Element];
}

/**
 * Renders an expandable entry.
 */
export function ExpandableEntry({
  expanded,
  sx,
  children,
}: ExpandableEntryProps) {
  return (
    <Box sx={sx}>
      <ExpandedContext.Provider value={expanded}>
        {children}
      </ExpandedContext.Provider>
    </Box>
  );
}
