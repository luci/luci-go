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
  Home as HomeIcon,
  MoreVert as MoreVertIcon,
} from '@mui/icons-material';
import {
  AppBar,
  Box,
  Divider,
  IconButton,
  Menu,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { Link as RouterLink, useLocation } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { genFeedbackUrl } from '@/common/tools/utils';
import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { CRYSTAL_BALL_ROUTES, isLandingPage } from '@/crystal_ball/routes';

import { useTopBar } from './top_bar_context';

/**
 * The top navigation bar for the CrystalBall application.
 * It displays the logo, page title, and actions based on the current context.
 */
export function TopBar() {
  const { title, actions, menuItems, subHeader } = useTopBar();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const location = useLocation();
  const authState = useAuthState();
  const isLoggedIn = authState.identity !== ANONYMOUS_IDENTITY;

  const isLanding = isLandingPage(location.pathname);

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
          <Tooltip title={COMMON_MESSAGES.CRYSTAL_BALL_DASHBOARDS}>
            <Box
              component={RouterLink}
              to={CRYSTAL_BALL_ROUTES.LANDING}
              sx={{
                textDecoration: 'none',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                color: 'primary.main',
              }}
            >
              <Typography
                variant="h6"
                component="div"
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  fontWeight: (theme) => theme.typography.fontWeightBold,
                  color: (theme) => theme.palette.primary.main,
                  cursor: 'pointer',
                  '&:hover': { color: (theme) => theme.palette.primary.dark },
                }}
              >
                {isLanding ? (
                  COMMON_MESSAGES.CRYSTAL_BALL_DASHBOARDS
                ) : (
                  <HomeIcon titleAccess="Home" />
                )}
              </Typography>
            </Box>
          </Tooltip>
          {title && (
            <>
              <Divider
                orientation="vertical"
                flexItem
                sx={{ mx: 2, height: 24, alignSelf: 'center' }}
              />
              <Typography variant="h6" sx={{ color: 'text.primary' }}>
                {title}
              </Typography>
            </>
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
