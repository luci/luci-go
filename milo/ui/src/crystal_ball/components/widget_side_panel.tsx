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

import {
  ChevronLeft as ChevronLeftIcon,
  ChevronRight as ChevronRightIcon,
  Close as CloseIcon,
} from '@mui/icons-material';
import {
  Box,
  Card,
  CardContent,
  CardHeader,
  Divider,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { ReactNode, useContext } from 'react';
import { createPortal } from 'react-dom';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { WidgetPortalContext } from '@/crystal_ball/context';

interface WidgetSidePanelProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
}

/**
 * A side panel component that renders its children inside a portal target provided by WidgetPortalContext.
 * It displays a title and a close button.
 */
export function WidgetSidePanel({
  title,
  children,
  onClose,
}: WidgetSidePanelProps) {
  const portalContext = useContext(WidgetPortalContext);

  if (!portalContext?.target) return null;

  if (portalContext.isFolded) {
    return createPortal(
      <Tooltip title="Unfold panel" placement="left">
        <Box
          onClick={portalContext.expand}
          sx={{
            width: '100%',
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            bgcolor: 'action.hover',
            cursor: 'pointer',
            gap: 2,
            '&:hover': {
              bgcolor: 'action.selected',
            },
          }}
        >
          <ChevronLeftIcon fontSize="small" sx={{ color: 'text.secondary' }} />
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 0.25,
            }}
          >
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: 'bold',
                textTransform: 'uppercase',
                letterSpacing: 0.5,
                lineHeight: 1,
                textAlign: 'center',
              }}
            >
              {COMMON_MESSAGES.INVOCATION_DETAILS}
            </Typography>
          </Box>
        </Box>
      </Tooltip>,
      portalContext.target,
    );
  }

  return createPortal(
    <Card
      variant="outlined"
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <CardHeader
        title={title}
        titleTypographyProps={{ variant: 'subtitle1', fontWeight: 'bold' }}
        action={
          <Box sx={{ display: 'flex', gap: 0.5 }}>
            <IconButton
              size="small"
              onClick={portalContext.fold}
              title="Fold panel"
            >
              <ChevronRightIcon fontSize="small" />
            </IconButton>
            <IconButton size="small" onClick={onClose} title="Close panel">
              <CloseIcon fontSize="small" />
            </IconButton>
          </Box>
        }
        sx={{ px: 2, py: 1.5 }}
      />
      <Divider />
      <CardContent
        sx={{
          p: 0,
          '&:last-child': { pb: 0 },
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {children}
      </CardContent>
    </Card>,
    portalContext.target,
  );
}
