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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  ListItem,
  ListItemText,
  Collapse,
  Box,
  IconButton,
} from '@mui/material';
import React from 'react';

interface ExpandableListItemProps {
  isExpanded: boolean;
  label: string;
  secondaryText?: string;
  children?: React.ReactNode;
  isSelected?: boolean;
  onClick?: () => void;
  level?: number;
  iconElement?: React.ReactNode;
  showBorder?: boolean;
}

export const ExpandableListItem: React.FC<ExpandableListItemProps> = ({
  isExpanded,
  label,
  secondaryText,
  children,
  isSelected,
  onClick,
  level = 0,
  iconElement,
  showBorder = false,
}) => {
  const paddingLeft = level * 3 + 1;

  return (
    <Box
      sx={{
        ...(showBorder && {
          borderTop: '1px solid #ccc',
        }),
      }}
    >
      <ListItem
        onClick={onClick}
        dense
        sx={{
          pl: children ? `${paddingLeft}px` : `${paddingLeft + 3}px`,
          py: 0,
          cursor: children ? 'pointer' : onClick ? 'pointer' : 'default',
        }}
      >
        {children ? (
          <IconButton size="small">
            {isExpanded ? (
              <ExpandMoreIcon fontSize="small" />
            ) : (
              <ChevronRightIcon
                fontSize="small"
                sx={{ transform: 'rotate(0deg)', color: 'text.secondary' }}
              />
            )}
          </IconButton>
        ) : (
          iconElement && (
            <IconButton size="small" sx={{ p: 0.25 }} disabled>
              {iconElement}
            </IconButton>
          )
        )}
        <ListItemText
          primary={label || 'Unknown'}
          secondary={secondaryText}
          title={label}
          sx={{ m: '2px 0' }}
          primaryTypographyProps={{
            sx: {
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              fontStyle: label ? 'normal' : 'italic',
              fontWeight: isSelected ? 'bold' : 'normal',
              marginLeft: children || iconElement ? '8px' : undefined,
            },
          }}
          secondaryTypographyProps={{
            variant: 'caption',
            sx: {
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              marginLeft: children || iconElement ? '8px' : undefined,
            },
          }}
        />
      </ListItem>
      {children && (
        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
          {children}
        </Collapse>
      )}
    </Box>
  );
};
