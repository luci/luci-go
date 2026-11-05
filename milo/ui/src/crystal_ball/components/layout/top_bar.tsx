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
  FeedbackOutlined as FeedbackIcon,
  MoreVert as MoreVertIcon,
} from '@mui/icons-material';
import {
  AppBar,
  Box,
  IconButton,
  Menu,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { genFeedbackUrl } from '@/common/tools/utils';
import { COMMON_MESSAGES } from '@/crystal_ball/constants';

import { useTopBar } from './top_bar_context';

/**
 * The page-level top bar for the CrystalBall application.
 * It displays the page title and actions based on the current context.
 *
 * App-level branding (the "CrystalBall" name and home link) is rendered by the
 * shared LUCI app bar, configured via the route handle in
 * `@/crystal_ball/routes`.
 */
export function TopBar() {
  const { title, actions, menuItems, subHeader } = useTopBar();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const authState = useAuthState();
  const isLoggedIn = authState.identity !== ANONYMOUS_IDENTITY;

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  return (
    <AppBar
      position="static"
      color="default"
      elevation={1}
      sx={{ bgcolor: 'white' }}
    >
      <Toolbar sx={{ justifyContent: 'space-between' }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          {title && (
            <Typography variant="h6" sx={{ color: 'text.primary' }}>
              {title}
            </Typography>
          )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {actions}
          {isLoggedIn && (
            <Tooltip title="Submit Feedback">
              <IconButton
                size="medium"
                aria-label="submit feedback"
                onClick={() =>
                  window.open(
                    genFeedbackUrl({ bugComponent: '2000424' }),
                    '_blank',
                  )
                }
                sx={{ color: 'action.active' }}
              >
                <FeedbackIcon />
              </IconButton>
            </Tooltip>
          )}
          {menuItems && (
            <>
              <Tooltip title={COMMON_MESSAGES.SHOW_MORE}>
                <IconButton
                  size="medium"
                  aria-label="show more"
                  aria-controls="menu-appbar"
                  aria-haspopup="true"
                  onClick={handleMenuClick}
                  sx={{ color: 'action.active' }}
                >
                  <MoreVertIcon />
                </IconButton>
              </Tooltip>
              <Menu
                id="menu-appbar"
                anchorEl={anchorEl}
                open={Boolean(anchorEl)}
                onClose={handleMenuClose}
              >
                {menuItems}
              </Menu>
            </>
          )}
        </Box>
      </Toolbar>
      {subHeader}
    </AppBar>
  );
}
