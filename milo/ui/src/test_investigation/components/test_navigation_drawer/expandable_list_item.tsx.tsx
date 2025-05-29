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

import UnfoldLessIcon from '@mui/icons-material/UnfoldLess';
import UnfoldMoreIcon from '@mui/icons-material/UnfoldMore';
import {
  ListItemText,
  Collapse,
  Box,
  IconButton,
  Typography,
  Chip,
  Divider,
  ListItemButton,
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
}

export const ExpandableListItem: React.FC<ExpandableListItemProps> = ({
  isExpanded,
  label,
  secondaryText,
  children,
  isSelected,
  onClick,
  level = 0,
}) => {
  const paddingLeft = level * 10 + 1;

  if (secondaryText?.startsWith('0 failed')) {
    secondaryText = secondaryText.replace('0 failed', 'Passed');
  }

  return (
    <Box sx={{ pt: 0 }}>
      <ListItemButton
        selected={isSelected}
        onClick={onClick}
        dense
        sx={{
          pl: children ? `${paddingLeft}px` : `${paddingLeft + 3}px`,
          py: 1.2,
          cursor: children ? 'pointer' : onClick ? 'pointer' : 'default',
        }}
      >
        {children && (
          <>
            {isExpanded ? (
              <IconButton size="small">
                <UnfoldLessIcon fontSize="small" />
              </IconButton>
            ) : (
              <IconButton size="small">
                <UnfoldMoreIcon fontSize="small" />
              </IconButton>
            )}
          </>
        )}
        <ListItemText
          primary={
            <Typography
              variant="subtitle1"
              color="primary"
              sx={{
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                fontStyle: label ? 'normal' : 'italic',
                marginLeft: children ? '8px' : undefined,
              }}
            >
              {label || 'Unknown'}
            </Typography>
          }
          secondary={
            secondaryText && (
              <Chip
                label={
                  <>
                    <Typography variant="caption">
                      {secondaryText.split('(')[0]}
                    </Typography>
                    {secondaryText.includes('(') && (
                      <Typography
                        variant="caption"
                        sx={{ fontStyle: 'italic' }}
                        color="grey"
                      >
                        {'(' + secondaryText.split('(')[1]}
                      </Typography>
                    )}
                  </>
                }
                sx={{
                  ml: children ? '8px' : undefined,
                }}
                color={secondaryText.startsWith('Passed') ? 'success' : 'error'}
              ></Chip>
            )
          }
          title={label}
          sx={{ m: '2px 0' }}
          disableTypography
        />
      </ListItemButton>
      <Divider component="li" />
      {children && (
        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
          {children}
        </Collapse>
      )}
    </Box>
  );
};
