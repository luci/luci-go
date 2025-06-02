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

import ArticleIcon from '@mui/icons-material/Article';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ImageIcon from '@mui/icons-material/Image';
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined';
import MoreHorizIcon from '@mui/icons-material/MoreHoriz';
import { Box, Typography, IconButton, Chip, Theme } from '@mui/material';
import { deepOrange, yellow, blue } from '@mui/material/colors';
import { useTheme } from '@mui/material/styles'; // Added
import React, { ReactNode } from 'react';

import { TreeData } from '@/common/components/log_viewer/virtual_tree/types';
import { VirtualTreeNodeActions } from '@/common/components/log_viewer/virtual_tree/virtual_tree';

import { ArtifactTreeNodeData } from '../types';

import { FolderIcon } from './folder_icon';

const LEVEL_INDENTATION_SIZE = 10; // Pixels per indentation level
const CONTENT_INTERNAL_OFFSET_LEFT = 8; // Base left padding for content

const DIRECT_ACTIVE_NODE_SELECTION_BACKGROUND_COLOR = deepOrange[300];
const DIRECT_SEARCH_MATCHED_BACKGROUND_COLOR = yellow[400];
/**
 * Determines the background color for a tree node based on its state.
 */
const getNodeBackground = (
  context: VirtualTreeNodeActions<any>, // eslint-disable-line @typescript-eslint/no-explicit-any
  theme: Theme,
): string => {
  if (context.isActiveSelection) {
    return DIRECT_ACTIVE_NODE_SELECTION_BACKGROUND_COLOR;
  }
  if (context.isSearchMatch) {
    return DIRECT_SEARCH_MATCHED_BACKGROUND_COLOR;
  }
  if (context.isSelected) {
    return blue[50];
  }
  return theme.palette.background.paper; // Default row background
};

/**
 * Extracts a displayable file type from a filename.
 */
function getFileTypeFromName(fileName: string): string | null {
  const lastDot = fileName.lastIndexOf('.');
  if (lastDot === -1 || lastDot === 0 || lastDot === fileName.length - 1) {
    return 'text';
  }
  const extension = fileName.substring(lastDot + 1).toLowerCase();

  if (
    extension === 'png' ||
    extension === 'jpg' ||
    extension === 'jpeg' ||
    extension === 'gif' ||
    extension === 'svg' ||
    extension === 'heic'
  ) {
    return extension;
  }
  if (extension.length > 0 && extension.length <= 4) {
    return extension;
  }
  return 'text'; // Fallback for unknown or long extensions
}

/**
 * Renders an appropriate icon for a leaf node based on its file type.
 */
const LeafFileIcon = ({ fileType }: { fileType: string | null }) => {
  const theme = useTheme(); // Access theme for icon color
  let IconComponent = InsertDriveFileOutlinedIcon; // Default file icon
  if (fileType) {
    switch (fileType) {
      case 'png':
      case 'jpg':
      case 'jpeg':
      case 'gif':
      case 'svg':
      case 'heic':
        IconComponent = ImageIcon;
        break;
      case 'text':
      case 'xml':
      case 'json':
      case 'yaml':
      case 'md':
        IconComponent = ArticleIcon;
        break;
    }
  }
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', height: '24px' }}>
      <IconComponent
        sx={{ fontSize: '20px', color: theme.palette.action.active }}
      />
    </Box>
  );
};

/**
 * Props for the CustomTreeNode component.
 * @template T Extends TreeNodeData, representing the specific data type for a node.
 */
export interface ArtifactTreeNodeProps {
  /** The index of the node in the virtualized list. */
  index: number;
  /** The processed data for the tree node, including level, isOpen state, etc. */
  row: TreeData<ArtifactTreeNodeData>;
  /** Context object containing actions (onNodeToggle, onNodeSelect) and state flags (isSelected, etc.). */
  context: VirtualTreeNodeActions<ArtifactTreeNodeData>;
  /** Optional function to render custom action icons/buttons for the node. */
  renderActions?: (row: TreeData<ArtifactTreeNodeData>) => ReactNode;
  /** Callback for when a "supported" (viewable internally) leaf node is clicked. */
  onSupportedLeafClick?: (nodeData: ArtifactTreeNodeData) => void;
  /** Callback for when an "unsupported" (opens externally) leaf node is clicked. */
  onUnsupportedLeafClick?: (nodeData: ArtifactTreeNodeData) => void;
}

export function ArtifactTreeNode({
  row,
  context,
  renderActions,
  onSupportedLeafClick,
  onUnsupportedLeafClick,
}: ArtifactTreeNodeProps) {
  const theme = useTheme();
  const isFolder = !row.isLeafNode;
  const backgroundColor = getNodeBackground(context, theme);
  const fileType = isFolder ? null : getFileTypeFromName(row.name);
  const totalPaddingLeft =
    row.level * LEVEL_INDENTATION_SIZE + CONTENT_INTERNAL_OFFSET_LEFT;

  const isUnsupportedLink =
    !isFolder && !row.data.viewingSupported && !row.data.isSummary;

  const handleToggle = () => {
    context.onNodeToggle?.(row);
  };

  const handleSelect = () => {
    context.onNodeSelect?.(row);

    if (row.isLeafNode && row.data) {
      if (row.data.viewingSupported || row.data.isSummary) {
        onSupportedLeafClick?.(row.data);
      } else {
        onUnsupportedLeafClick?.(row.data);
      }
    }
  };

  const handleDefaultActionClick = (event: React.MouseEvent) => {
    event.stopPropagation();
  };

  return (
    <Box
      sx={{
        width: '100%',
        minHeight: '40px',
        backgroundColor: backgroundColor,
        borderBottom: `1px solid ${theme.palette.divider}`,
        display: 'flex',
        alignItems: 'center',
        paddingLeft: `${totalPaddingLeft}px`,
        paddingRight: '8px',
        boxSizing: 'border-box',
        cursor: 'pointer',
        '&:hover': {
          backgroundColor: context.isSelected
            ? theme.palette.grey[200]
            : theme.palette.grey[100],
        },
      }}
      onClick={handleSelect}
      role="treeitem"
      aria-level={row.level + 1}
      aria-expanded={isFolder ? row.isOpen : undefined}
      aria-selected={context.isSelected}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
          width: '100%',
          minHeight: '24px',
        }}
      >
        <Box
          sx={{
            width: '24px',
            height: '24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
          }}
        >
          {isFolder && (
            <IconButton
              size="small"
              onClick={handleToggle}
              aria-label={row.isOpen ? 'Collapse node' : 'Expand node'}
              sx={{ padding: 0 }}
            >
              {row.isOpen ? (
                <ExpandLessIcon sx={{ fontSize: '24px' }} />
              ) : (
                <ChevronRightIcon
                  sx={{ fontSize: '24px', color: theme.palette.action.active }}
                />
              )}
            </IconButton>
          )}
        </Box>

        <Box
          sx={{
            width: '24px',
            height: '24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
          }}
        >
          {isFolder ? (
            <FolderIcon sx={{ fontSize: '24px' }} />
          ) : (
            <LeafFileIcon fileType={fileType} />
          )}
        </Box>

        {!isFolder && fileType && (
          <Chip
            label={fileType}
            size="small"
            sx={{
              height: '18px',
              fontSize: '11px',
              lineHeight: '16px',
              backgroundColor: theme.palette.grey[200], // Chip background
              color: theme.palette.text.secondary, // Chip text
              borderRadius: '4px',
              mr: '4px',
              flexShrink: 0,
            }}
          />
        )}

        <Typography
          variant="body2"
          sx={{
            fontFamily: 'Roboto, sans-serif',
            fontWeight: 400,
            fontSize: '14px',
            lineHeight: '20px',
            letterSpacing: '0.2px',
            color: isUnsupportedLink
              ? theme.palette.primary.main
              : theme.palette.text.primary, // Link and default text
            textDecoration: isUnsupportedLink ? 'underline' : 'none',
            flexGrow: 1,
            wordBreak: 'break-word',
          }}
        >
          {row.name}
        </Typography>

        <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
          {renderActions ? (
            renderActions(row)
          ) : row.isLeafNode ? (
            <IconButton
              size="small"
              aria-label={`Actions for ${row.name}`}
              onClick={handleDefaultActionClick}
              sx={{
                visibility: 'hidden',
                '.MuiBox-root:hover > .MuiBox-root > &': {
                  // Consider revising selector if Box structure changes
                  visibility: 'visible',
                },
              }}
            >
              <MoreHorizIcon sx={{ color: theme.palette.action.active }} />
            </IconButton>
          ) : (
            <Box sx={{ width: '24px', height: '24px' }} />
          )}
        </Box>
      </Box>
    </Box>
  );
}
