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
import DescriptionIcon from '@mui/icons-material/Description';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FolderIcon from '@mui/icons-material/Folder';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import {
  Box,
  IconButton,
  ListItemButton,
  ListItemIcon,
  ListItemText,
} from '@mui/material';

import { CustomArtifactTreeNode } from './types';

interface ArtifactTreeViewProps {
  node: CustomArtifactTreeNode;
  selectedNodeId: string | null;
  onNodeSelect: (node: CustomArtifactTreeNode) => void;
  expandedNodeIds: Set<string>;
  onNodeToggle: (nodeId: string) => void;
}

export function ArtifactTreeView({
  node,
  selectedNodeId,
  onNodeSelect,
  expandedNodeIds,
  onNodeToggle,
}: ArtifactTreeViewProps): JSX.Element {
  const isExpandable = !!node.children && node.children.length > 0;
  const isExpanded = expandedNodeIds.has(node.id);

  const handleToggle = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (isExpandable) {
      onNodeToggle(node.id);
    }
  };

  const handleSelect = () => {
    if (node.isSummary || node.isLeaf) {
      onNodeSelect(node);
    } else if (isExpandable) {
      onNodeToggle(node.id);
    }
  };

  let visualIcon = null;
  if (node.isRootChild || (isExpandable && !node.isLeaf)) {
    visualIcon = isExpanded ? (
      <FolderOpenIcon sx={{ fontSize: 18 }} />
    ) : (
      <FolderIcon sx={{ fontSize: 18 }} />
    );
  } else {
    visualIcon = node.isSummary ? (
      <ArticleIcon sx={{ fontSize: 18 }} />
    ) : (
      <DescriptionIcon sx={{ fontSize: 18 }} />
    );
  }

  return (
    <>
      <ListItemButton
        dense
        onClick={handleSelect}
        selected={selectedNodeId === node.id}
        sx={{
          pl: node.level * 2 + (node.isRootChild || node.level === 0 ? 1 : 3),
          py: 0.15,
          display: 'flex',
          alignItems: 'center',
        }}
      >
        {(isExpandable ||
          (node.isRootChild && node.children && node.children.length > 0)) && (
          <ListItemIcon
            sx={{
              minWidth: 'auto',
              mr: 0.5,
              display: 'flex',
              alignItems: 'center',
              width: 24,
              height: 24,
            }}
          >
            {isExpandable ? (
              <IconButton
                size="small"
                onClick={handleToggle}
                sx={{ p: 0.25 }}
                aria-label={isExpanded ? 'Collapse' : 'Expand'}
              >
                {isExpanded ? (
                  <ExpandMoreIcon fontSize="small" />
                ) : (
                  <ChevronRightIcon fontSize="small" />
                )}
              </IconButton>
            ) : (
              <Box sx={{ width: 24, height: 24 }} />
            )}
          </ListItemIcon>
        )}
        <Box
          sx={{
            mr: 0.5,
            display: 'flex',
            alignItems: 'center',
            pl:
              isExpandable ||
              (node.isRootChild && node.children && node.children.length > 0)
                ? 0
                : node.level === 0 && !isExpandable
                  ? 0
                  : 2.5,
          }}
        >
          {visualIcon}
        </Box>
        <ListItemText
          primary={node.name}
          primaryTypographyProps={{
            variant: 'body2',
            sx: {
              fontWeight: selectedNodeId === node.id ? 'bold' : 'normal',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            },
          }}
        />
      </ListItemButton>
      {isExpandable && isExpanded && node.children && (
        <Box>
          {node.children.map((childNode) => (
            <ArtifactTreeView
              key={childNode.id}
              node={childNode}
              selectedNodeId={selectedNodeId}
              onNodeSelect={onNodeSelect}
              expandedNodeIds={expandedNodeIds}
              onNodeToggle={onNodeToggle}
            />
          ))}
        </Box>
      )}
    </>
  );
}
