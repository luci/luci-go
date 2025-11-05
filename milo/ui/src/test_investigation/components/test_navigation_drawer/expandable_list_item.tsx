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
import React, { RefObject } from 'react';

import { getStatusStyle } from '@/common/styles/status_styles';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { getSemanticStatusFromVerdict } from '@/test_investigation/utils/drawer_tree_utils';

interface ExpandableListItemProps {
  isExpanded: boolean;
  label: string;
  status?: TestVerdict_Status; // Undefined for non-leaf nodes.
  children?: React.ReactNode;
  secondaryText?: string;
  totalTests: number;
  isSelected?: boolean;
  onClick?: () => void;
  level?: number;
  itemRef?: RefObject<HTMLDivElement | null>;
}

export const ExpandableListItem: React.FC<ExpandableListItemProps> = ({
  isExpanded,
  label,
  children,
  status,
  secondaryText,
  totalTests,
  isSelected,
  onClick,
  level = 0,
  itemRef,
}) => {
  const paddingLeft = level * 10 + 1;

  // For leaf nodes, just get the correct style for the test verdict status.
  const style = status
    ? getStatusStyle(getSemanticStatusFromVerdict(status))
    : undefined;

  return (
    <Box sx={{ pt: 0 }} ref={itemRef}>
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
            status &&
            secondaryText && (
              <Chip
                label={
                  <Box
                    sx={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 0,
                      my: '4px',
                    }}
                  >
                    <Typography variant="caption">
                      {secondaryText.charAt(0).toUpperCase() +
                        secondaryText.slice(1).toLowerCase()}
                    </Typography>
                    {children && (
                      <Typography
                        variant="caption"
                        sx={{ fontStyle: 'italic', color: 'grey' }}
                      >
                        {totalTests} tests ran
                      </Typography>
                    )}
                  </Box>
                }
                sx={{
                  ml: children ? '8px' : undefined,
                  backgroundColor: style?.backgroundColor || 'warning',
                  height: children ? '40px' : '24px',
                  borderRadius: '4px',
                  '& .MuiChip-label': { px: '8px' },
                }}
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
