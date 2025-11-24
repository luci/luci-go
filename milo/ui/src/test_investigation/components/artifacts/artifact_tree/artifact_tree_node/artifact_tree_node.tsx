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

import AdbIcon from '@mui/icons-material/Adb';
import ArticleIcon from '@mui/icons-material/Article';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ImageIcon from '@mui/icons-material/Image';
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined';
import {
  Box,
  Typography,
  IconButton,
  Chip,
  Theme,
  Tooltip,
  Button,
  Link,
} from '@mui/material';
import { deepOrange, yellow, blue } from '@mui/material/colors';
import { useTheme } from '@mui/material/styles';
import { ReactNode } from 'react';

import { TreeData } from '@/common/components/log_viewer/virtual_tree/types';
import { VirtualTreeNodeActions } from '@/common/components/log_viewer/virtual_tree/virtual_tree';
import { getAndroidBugToolLink } from '@/common/tools/url_utils';
import { useInvocation } from '@/test_investigation/context';
import { isAnTSInvocation } from '@/test_investigation/utils/test_info_utils';

import { ArtifactTreeNodeData } from '../../types';
import { getArtifactType } from '../artifact_utils';

import { FolderIcon } from './folder_icon';

const LEVEL_INDENTATION_SIZE = 10;
const CONTENT_INTERNAL_OFFSET_LEFT = 8;

const DIRECT_ACTIVE_NODE_SELECTION_BACKGROUND_COLOR = deepOrange[300];
const DIRECT_SEARCH_MATCHED_BACKGROUND_COLOR = yellow[400];

function renderHighlightedText(
  text: string,
  highlight: string,
): React.ReactNode {
  if (!highlight.trim()) {
    return text;
  }
  const parts = text.split(new RegExp(`(${highlight})`, 'gi'));
  return (
    <span>
      {parts.map((part, index) =>
        part.toLowerCase() === highlight.toLowerCase() ? (
          <Box
            component="span"
            key={index}
            sx={{
              backgroundColor: yellow[200],
              borderRadius: '2px',
            }}
          >
            <strong>{part}</strong>
          </Box>
        ) : (
          part
        ),
      )}
    </span>
  );
}

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
  return theme.palette.background.paper;
};

const LeafFileIcon = ({ fileType }: { fileType: string | null }) => {
  const theme = useTheme();
  let IconComponent = InsertDriveFileOutlinedIcon;
  let titleAccess: string = 'file-icon';
  if (fileType) {
    switch (fileType) {
      case 'image':
      case 'png':
      case 'jpg':
      case 'jpeg':
      case 'gif':
      case 'svg':
      case 'heic':
        IconComponent = ImageIcon;
        titleAccess = 'image-icon';
        break;
      case 'text':
      case 'xml':
      case 'json':
      case 'yaml':
      case 'md':
      case 'log':
        IconComponent = ArticleIcon;
        titleAccess = 'article-icon';
        break;
    }
  }
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', height: '24px' }}>
      <IconComponent
        sx={{ fontSize: '20px', color: theme.palette.action.active }}
        titleAccess={titleAccess}
      />
    </Box>
  );
};

export interface ArtifactTreeNodeProps {
  index: number;
  row: TreeData<ArtifactTreeNodeData>;
  context: VirtualTreeNodeActions<ArtifactTreeNodeData>;
  renderActions?: (row: TreeData<ArtifactTreeNodeData>) => ReactNode;
  onSupportedLeafClick?: (nodeData: ArtifactTreeNodeData) => void;
  onUnsupportedLeafClick?: (nodeData: ArtifactTreeNodeData) => void;
  highlightText?: string;
}

export function ArtifactTreeNode({
  row,
  context,
  renderActions,
  onSupportedLeafClick,
  onUnsupportedLeafClick,
  highlightText,
}: ArtifactTreeNodeProps) {
  const theme = useTheme();
  const isFolder = !row.isLeafNode;
  const backgroundColor = getNodeBackground(context, theme);
  const artifactType = isFolder
    ? null
    : row.data.artifact?.artifactType || getArtifactType(row.name);
  const invocation = useInvocation();
  const isAnTS = isAnTSInvocation(invocation.name);

  const totalPaddingLeft =
    row.level * LEVEL_INDENTATION_SIZE + CONTENT_INTERNAL_OFFSET_LEFT;

  const isUnsupportedLink =
    !isFolder && !row.data.viewingSupported && !row.data.isSummary;

  const handleToggle = (event: React.MouseEvent) => {
    event.stopPropagation();
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
            <FolderIcon sx={{ fontSize: '24px' }} titleAccess="folder-icon" />
          ) : (
            <LeafFileIcon fileType={artifactType} />
          )}
        </Box>

        {!isFolder && artifactType && artifactType !== 'file' && (
          <Chip
            label={artifactType}
            size="small"
            sx={{
              height: '18px',
              fontSize: '11px',
              lineHeight: '16px',
              backgroundColor: theme.palette.grey[200],
              color: theme.palette.text.secondary,
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
              : theme.palette.text.primary,
            textDecoration: isUnsupportedLink ? 'underline' : 'none',
            flexGrow: 1,
            wordBreak: 'break-word',
          }}
        >
          {highlightText
            ? renderHighlightedText(row.name, highlightText)
            : row.name}
        </Typography>

        <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
          {renderActions ? (
            renderActions(row)
          ) : row.isLeafNode && isAnTS && row.data.artifact?.artifactId ? (
            <Tooltip title="Open in Android Bug Tool">
              <Button
                component={Link}
                size="small"
                target="_blank"
                aria-label={`Actions for ${row.name}`}
                href={getAndroidBugToolLink(
                  row.data.artifact.artifactId,
                  invocation.name,
                )}
              >
                <AdbIcon
                  sx={{ color: theme.palette.action.active }}
                  titleAccess="adb-icon"
                />
              </Button>
            </Tooltip>
          ) : (
            <Box sx={{ width: '24px', height: '24px' }} />
          )}
        </Box>
      </Box>
    </Box>
  );
}
