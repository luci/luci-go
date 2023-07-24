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

import FeedbackIcon from '@mui/icons-material/Feedback';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import SettingsIcon from '@mui/icons-material/Settings';
import IconButton from '@mui/material/IconButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import { observer } from 'mobx-react-lite';
import { useState, MouseEvent } from 'react';

import { useStore } from '@/common/store';
import { genFeedbackUrl } from '@/common/tools/utils';

export const SettingsMenu = observer(() => {
  const store = useStore();

  const [anchorElUser, setAnchorElUser] = useState<null | HTMLElement>(null);

  const handleOpenUserMenu = (event: MouseEvent<HTMLElement>) => {
    setAnchorElUser(event.currentTarget);
  };

  const handleCloseUserMenu = () => {
    setAnchorElUser(null);
  };
  return (
    <>
      <IconButton
        sx={{ color: 'white' }}
        onClick={() => window.open(genFeedbackUrl())}
        size="medium"
        role="button"
        aria-label="open settings menu"
      >
        <FeedbackIcon />
      </IconButton>

      <IconButton color="inherit" onClick={handleOpenUserMenu}>
        <MoreVertIcon />
      </IconButton>
      <Menu
        sx={{ mt: 4 }}
        id="menu-appbar"
        anchorEl={anchorElUser}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        keepMounted
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        open={Boolean(anchorElUser)}
        onClose={handleCloseUserMenu}
      >
        <MenuItem
          onClick={() => store.setShowSettingsDialog(true)}
          disabled={store.hasSettingsDialog === 0}
          title="Change settings specific to the page."
        >
          <ListItemIcon>
            <SettingsIcon />
          </ListItemIcon>
          <ListItemText>Page Settings</ListItemText>
        </MenuItem>
      </Menu>
    </>
  );
});
