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

import HomeIcon from '@mui/icons-material/Home';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import Menu from '@mui/material/Menu';
import Toolbar from '@mui/material/Toolbar';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { Link as RouterLink, useLocation } from 'react-router';

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
          <Tooltip title="CrystalBall Dashboards">
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
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {actions}
          {menuItems && (
            <>
              <IconButton
                size="large"
                aria-label="show more"
                aria-controls="menu-appbar"
                aria-haspopup="true"
                onClick={handleMenuClick}
                color="inherit"
              >
                <MoreVertIcon />
              </IconButton>
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
