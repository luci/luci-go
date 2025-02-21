// Copyright 2024 The LUCI Authors.
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

import LaunchIcon from '@mui/icons-material/Launch';
import MenuIcon from '@mui/icons-material/Menu';
import { styled, Typography } from '@mui/material';
import MuiDrawer from '@mui/material/Drawer';
import MaterialLink from '@mui/material/Link';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import ListSubheader from '@mui/material/ListSubheader';
import Toolbar from '@mui/material/Toolbar';
import { Fragment, useMemo } from 'react';
import { Link, useLocation } from 'react-router-dom';

import { colors } from '@/fleet/theme/colors';

import { DRAWER_WIDTH } from './constants';
import { generateSidebarSections } from './sidebar_sections';

const Drawer = styled(MuiDrawer)(({ theme, open }) => ({
  width: 0,
  transition: theme.transitions.create('width', {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.leavingScreen,
  }),
  ...(open && {
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
    width: `${DRAWER_WIDTH}px`,
  }),
}));

export const Sidebar = ({ open }: { open: boolean }) => {
  const location = useLocation();
  const sidebarSections = useMemo(() => {
    return generateSidebarSections();
  }, []);

  return (
    <Drawer
      sx={{
        position: 'relative',
        height: '100%',
        '& .MuiDrawer-paper': {
          width: DRAWER_WIDTH,
          boxSizing: 'border-box',
        },
      }}
      variant="persistent"
      anchor="left"
      open={open}
      role="complementary"
    >
      <Toolbar variant="dense" sx={{ height: 64 }} />
      <List sx={{ mb: '40px', pt: 0 }}>
        {sidebarSections.map((sidebarSection) => (
          <Fragment key={sidebarSection.title}>
            <ListSubheader>{sidebarSection.title}</ListSubheader>
            {sidebarSection.pages.map((sidebarPage) => (
              <ListItem
                dense
                key={sidebarPage.url}
                disablePadding
                sx={{ display: 'block' }}
              >
                <ListItemButton
                  sx={{ px: 2.5 }}
                  selected={sidebarPage.url === location.pathname}
                  component={sidebarPage.external ? MaterialLink : Link}
                  to={sidebarPage.url}
                  target={sidebarPage.external ? '_blank' : ''}
                >
                  <ListItemIcon sx={{ minWidth: 0, mr: 1 }}>
                    {sidebarPage.icon}
                  </ListItemIcon>

                  <ListItemText primary={sidebarPage.label} />

                  {sidebarPage.external && (
                    <ListItemIcon sx={{ minWidth: 0, mr: 1 }}>
                      <LaunchIcon color="inherit" />
                    </ListItemIcon>
                  )}
                </ListItemButton>
              </ListItem>
            ))}
          </Fragment>
        ))}

        <div style={{ margin: '40px 20px', color: colors.grey[600] }}>
          <Typography variant="caption">
            Click{' '}
            <MenuIcon
              fontSize="small"
              style={{ position: 'relative', top: '5px' }}
            />{' '}
            above to hide this menu.
          </Typography>
        </div>
      </List>
    </Drawer>
  );
};
