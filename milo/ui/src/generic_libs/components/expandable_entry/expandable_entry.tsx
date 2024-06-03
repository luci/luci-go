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
import { createContext, useContext, useRef } from 'react';

const ExpandedContext = createContext(false);

export interface ExpandableEntryHeaderProps {
  readonly onToggle: (expand: boolean) => void;
  readonly disabled?: boolean;
  readonly title?: string;
  readonly sx?: SxProps<Theme>;
  readonly children: React.ReactNode;
}

/**
 * Renders the header of an <ExpandableEntry />.
 */
export function ExpandableEntryHeader({
  onToggle,
  disabled = false,
  title,
  sx,
  children,
}: ExpandableEntryHeaderProps) {
  const expanded = useContext(ExpandedContext);

  return (
    <Box
      onClick={() => disabled || onToggle(!expanded)}
      title={title}
      sx={{
        display: 'grid',
        gridTemplateColumns: '24px 1fr',
        gridGap: '5px',
        cursor: disabled ? 'unset' : 'pointer',
        color: (theme) => (disabled ? theme.palette.action.disabled : 'unset'),
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
  /**
   * Configure when the children should be rendered to DOM.
   * * was-expanded: the default option. Renders the children once the entry is
   *   expanded. Collapsing it does not cause the children to be removed from
   *   DOM. This is avoid unnecessary (re)rendering and can avoid the children's
   *   state being reset.
   * * always: always render the children to DOM. This can be useful when the
   *   children have some side effects that need to be triggered as soon as
   *   possible (e.g. a fetch query).
   * * expanded: only render the children to DOM when the entry is expanded.
   *   This can be useful when you want to reset internal state of the children
   *   after collapsing the entry.
   */
  readonly renderChildren?: 'was-expanded' | 'always' | 'expanded';
  readonly sx?: SxProps<Theme>;
  readonly children: React.ReactNode;
}

/**
 * Renders the body of an <ExpandableEntry />.
 * The content is hidden when the entry is collapsed.
 */
export function ExpandableEntryBody({
  ruler = 'visible',
  renderChildren = 'was-expanded',
  sx,
  children,
}: ExpandableEntryBodyProps) {
  const expanded = useContext(ExpandedContext);

  // We do not need `wasExpanded` to be a state because it could only change
  // when another state, `expanded`, is updated. Keeping it in a ref reduces
  // 1 rerendering cycle.
  const wasExpandedRef = useRef(expanded);
  wasExpandedRef.current ||= expanded;

  const shouldMount =
    expanded ||
    renderChildren === 'always' ||
    (renderChildren === 'was-expanded' && wasExpandedRef.current);

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
      {shouldMount && (
        // Use a Box isolate the children from grid layout.
        <Box sx={sx} hidden={!expanded}>
          {children}
        </Box>
      )}
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
