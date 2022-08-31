// Copyright 2022 The LUCI Authors.
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
  useContext,
  useMemo,
} from 'react';
import {
  Link,
  useParams,
} from 'react-router-dom';

import RuleIcon from '@mui/icons-material/Rule';
import SpokeIcon from '@mui/icons-material/Spoke';
import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
// eslint-disable-next-line import/no-unresolved
import { OverridableComponent } from '@mui/material/OverridableComponent';
import { SvgIconTypeMap } from '@mui/material/SvgIcon';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';

import { DynamicComponentNoProps } from '../../tools/rendering_tools';
import CollapsedMenu from './collapsed_menu/collapsed_menu';
import Logo from './logo/logo';
import {
  TopBarContext,
  TopBarContextProvider,
} from './top_bar_context';
import UserActions from './user_actions/user_actions';

type AppBarPageTitle = 'Clusters' | 'Rules';

export interface AppBarPage {
  title: AppBarPageTitle;
  url: string;
  icon: OverridableComponent<SvgIconTypeMap>;
}

function generatePages(projectId: string | undefined): AppBarPage[] {
  if (!projectId) {
    return [];
  }
  return [
    {
      title: 'Clusters',
      url: `/p/${projectId}/clusters`,
      icon: SpokeIcon,
    },
    {
      title: 'Rules',
      url: `/p/${projectId}/rules`,
      icon: RuleIcon,
    },
  ];
}

const TopBar = () => {
  const { setAnchorElNav } = useContext(TopBarContext);

  const handleCloseNavMenu = () => {
    setAnchorElNav(null);
  };

  const { project: projectId } = useParams();

  const pages = useMemo<AppBarPage[]>(() => generatePages(projectId), [projectId]);

  return (
    <TopBarContextProvider>
      <AppBar position="static">
        <Toolbar >
          <Box sx={{
            display: {
              xs: 'none',
              md: 'flex',
            },
            mr: 1,
            width: '3rem',
          }}>
            <Logo />
          </Box>
          <Typography
            variant="h6"
            noWrap
            component="a"
            href="/"
            sx={{
              mr: 2,
              display: { xs: 'none', md: 'flex' },
              color: 'inherit',
              textDecoration: 'none',
            }}
          >
            LUCI Analysis
          </Typography>
          <CollapsedMenu pages={pages}/>
          <Box sx={{ flexGrow: 1, display: { xs: 'none', md: 'flex' } }}>
            {pages.map((page) => (
              <Button
                component={Link}
                to={page.url}
                key={page.title}
                onClick={handleCloseNavMenu}
                sx={{ color: 'white' }}
                startIcon={<DynamicComponentNoProps component={page.icon}/>}
              >
                {page.title}
              </Button>
            ))}
          </Box>
          <UserActions />
        </Toolbar>
      </AppBar>
    </TopBarContextProvider>
  );
};

export default TopBar;
