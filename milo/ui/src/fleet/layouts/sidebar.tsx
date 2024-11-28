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

import BiotechOutlinedIcon from '@mui/icons-material/BiotechOutlined';
import DevicesIcon from '@mui/icons-material/Devices';
import LaunchIcon from '@mui/icons-material/Launch';
import MuiDrawer from '@mui/material/Drawer';
import MaterialLink from '@mui/material/Link';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import ListSubheader from '@mui/material/ListSubheader';
import Toolbar from '@mui/material/Toolbar';
import { Fragment } from 'react';
import { Link, useLocation } from 'react-router-dom';
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
    <MuiDrawer
      sx={{
        '& .MuiDrawer-paper': {
          width: 250,
          boxSizing: 'border-box',
        },
      }}
      variant="persistent"
      anchor="left"
      open={open}
      role="complementary"
    >
      <Toolbar variant="dense" />
      <List sx={{ mb: '40px', pt: 0 }}>
        {sidebarSections.map((sidebarSection) => (
          <Fragment key={sidebarSection.title}>
            <ListSubheader>{sidebarSection.title}</ListSubheader>
            {sidebarSection.pages.map((sidebarPage, idx) => (
              <ListItem
                dense
                key={sidebarPage.url + idx + sidebarSection.title}
                disablePadding
                sx={{ display: 'block' }}
              >
                <ListItemButton
                  sx={{
                    justifyContent: 'center',
                    px: 2.5,
                  }}
                  selected={sidebarPage.url === location.pathname}
                  component={sidebarPage.external ? MaterialLink : Link}
                  to={sidebarPage.url}
                  target={sidebarPage.external ? '_blank' : ''}
                >
                  <ListItemIcon
                    sx={{
                      minWidth: 0,
                      mr: 1,
                      justifyContent: 'center',
                    }}
                  >
                    {sidebarPage.icon}
                  </ListItemIcon>
                  <ListItemText primary={sidebarPage.label} />

                  {sidebarPage.external && (
                    <ListItemIcon
                      sx={{
                        minWidth: 0,
                        mr: 1,
                        justifyContent: 'center',
                      }}
                    >
                      <LaunchIcon color="inherit" />
                    </ListItemIcon>
                  )}
                </ListItemButton>
              </ListItem>
            ))}
          </Fragment>
        ))}
      </List>
    </MuiDrawer>
  );
};

/*

I like HSL for one off colors, I think the ideal solution will be to use [the colors specified by android T&D desings](https://carbon.googleplex.com/android-tools-and-data/pages/color/implimentation?edit=true) for most things and access them via MUI (I.E. `color.red[500]`)



 */
