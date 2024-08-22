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

import MenuIcon from '@mui/icons-material/Menu';
import { Link } from '@mui/material';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';
import { Link as RouterLink } from 'react-router-dom';

import { useSelectedPage, useProject } from '@/common/components/page_meta';
import { getProjectURLPath } from '@/common/tools/url_utils';

import { PAGE_LABEL_MAP } from '../../constants';

interface Props {
  open: boolean;
  handleSidebarChanged: (isOpen: boolean) => void;
}

export const AppDetails = ({ open, handleSidebarChanged }: Props) => {
  const selectedPage = useSelectedPage();
  const project = useProject();
  return (
    <>
      <IconButton
        size="large"
        edge="start"
        color="inherit"
        aria-label="menu"
        sx={{ mr: 2 }}
        onClick={() => handleSidebarChanged(!open)}
      >
        <MenuIcon />
      </IconButton>
      <Box
        sx={{
          display: {
            md: 'flex',
          },
          mr: 1,
          width: '2.5rem',
        }}
      >
        <Link component={RouterLink} to="/ui/">
          <img
            style={{ width: '100%' }}
            alt="logo"
            id="luci-icon"
            src="https://storage.googleapis.com/chrome-infra/lucy-small.png"
          />
        </Link>
      </Box>
      <Box sx={{ display: 'flex' }}>
        <Link
          component={RouterLink}
          to="/ui/"
          underline="none"
          variant="h6"
          sx={{ pr: 2, color: 'inherit' }}
        >
          LUCI
        </Link>
        {project && (
          <>
            <Divider flexItem orientation="vertical" sx={{ mr: 2 }} />
            <Link
              component={RouterLink}
              to={getProjectURLPath(project)}
              // Make it obvious that this link is clickable during the
              // transition phase of redesigned build page top bar.
              // We can decide whether we want to keep this style later.
              underline="always"
              variant="h6"
              sx={{ pr: 2, color: 'inherit', textDecorationColor: 'inherit' }}
            >
              {project}
            </Link>
          </>
        )}
        {selectedPage && (
          <>
            <Divider flexItem orientation="vertical" sx={{ mr: 2 }} />
            <Typography variant="h6" component="div" sx={{ pr: 2 }}>
              {PAGE_LABEL_MAP[selectedPage]}
            </Typography>
          </>
        )}
      </Box>
    </>
  );
};
