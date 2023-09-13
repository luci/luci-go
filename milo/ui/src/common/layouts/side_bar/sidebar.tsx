// Copyright 2023 The LUCI Authors.
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
import { Typography } from '@mui/material';
import Drawer from '@mui/material/Drawer';
import MaterialLink from '@mui/material/Link';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import ListSubheader from '@mui/material/ListSubheader';
import Toolbar from '@mui/material/Toolbar';
import { Fragment, useMemo } from 'react';
import { Link } from 'react-router-dom';

import {
  useSelectedPage,
  useProject,
} from '@/common/components/page_meta/page_meta_provider';

import { drawerWidth } from '../constants';

import { generateSidebarPages as generateSidebarSections } from './pages';

interface Props {
  open: boolean;
}

export const Sidebar = ({ open }: Props) => {
  const project = useProject();
  const selectedPage = useSelectedPage();

  const sidebarSections = useMemo(() => {
    return generateSidebarSections(project);
  }, [project]);

  return (
    <Drawer
      sx={{
        width: drawerWidth,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: drawerWidth,
          boxSizing: 'border-box',
        },
      }}
      variant="persistent"
      anchor="left"
      open={open}
      role="complementary"
    >
      <Toolbar variant="dense" />
      <List sx={{ mb: '40px' }}>
        {sidebarSections.map((sidebarSection) => (
          <Fragment key={sidebarSection.title}>
            <ListSubheader>{sidebarSection.title}</ListSubheader>
            {sidebarSection.pages.map((sidebarPage) => (
              <ListItem
                dense
                key={sidebarPage.page}
                disablePadding
                sx={{ display: 'block' }}
              >
                <ListItemButton
                  sx={{
                    justifyContent: 'center',
                    px: 2.5,
                  }}
                  selected={sidebarPage.page === selectedPage}
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
                  <ListItemText primary={sidebarPage.page} />

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
        <div style={{ margin: '40px 20px', color: '#888' }}>
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
