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

import DevicesIcon from '@mui/icons-material/Devices';
import { styled, Typography } from '@mui/material';
import MuiDrawer from '@mui/material/Drawer';
import MaterialLink from '@mui/material/Link';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import Toolbar from '@mui/material/Toolbar';
import { Fragment } from 'react';
import { Link, useLocation } from 'react-router-dom';

import { colors } from '../theme/colors';

import { drawerWidth } from './constants';

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
    width: `${drawerWidth}px`,
  }),
}));

type SidebarSection = {
  title: string;
  pages: {
    external?: boolean;
    url: string;
    icon: React.ReactNode;
    label: string;
  }[];
};

const sidebarSections: SidebarSection[] = [
  {
    title: 'Fleet Console',
    pages: [
      {
        url: '/ui/fleet/labs/devices',
        icon: <DevicesIcon />,
        label: 'Devices',
      },
    ],
  },
];

export const Sidebar = ({ open }: { open: boolean }) => {
  const location = useLocation();
  return (
    <Drawer
      sx={{
        position: 'relative',
        height: '100%',
        '& .MuiDrawer-paper': {
          width: drawerWidth,
          boxSizing: 'border-box',
        },
      }}
      variant="persistent"
      anchor="left"
      open={open}
      role="complementary"
      PaperProps={{
        elevation: 2,
      }}
    >
      <Toolbar
        variant="dense"
        sx={{
          height: 64,
        }}
      />
      <List sx={{ mb: '40px', pt: 0, margin: '20px 0' }}>
        {sidebarSections.map((sidebarSection) => (
          <Fragment key={sidebarSection.title}>
            {sidebarSection.pages.map((sidebarPage, idx) => (
              <ListItemButton
                key={sidebarPage.url + idx + sidebarSection.title}
                sx={{
                  px: 2.5,
                  borderRadius: '40px 999em 999em 40px',
                  // margin: '5px 0',
                  '&.Mui-selected': {
                    color: colors.blue[700],
                    backgroundColor: colors.blue[50],
                    ':hover': {
                      backgroundColor: colors.blue[100],
                    },
                  },
                }}
                selected={sidebarPage.url === location.pathname}
                component={sidebarPage.external ? MaterialLink : Link}
                to={sidebarPage.url}
                target={sidebarPage.external ? '_blank' : ''}
              >
                <Typography>{sidebarPage.label}</Typography>
              </ListItemButton>
            ))}
          </Fragment>
        ))}
      </List>
    </Drawer>
  );
};
