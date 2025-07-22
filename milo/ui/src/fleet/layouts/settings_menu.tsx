// Copyright 2025 The LUCI Authors.
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

import { MoreVert } from '@mui/icons-material';
import {
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
} from '@mui/material';
import { MouseEvent, useState } from 'react';

import {
  VersionControlIcon,
  useSwitchVersion,
  ROLLBACK_DURATION_WEEK,
} from '@/common/components/version_control';

const isNewUI = UI_VERSION_TYPE === 'new-ui';

export function SettingsMenu() {
  const switchVersion = useSwitchVersion();
  const switchVersionTitle = isNewUI
    ? `Switch to the LUCI UI version released ${ROLLBACK_DURATION_WEEK} weeks ago.`
    : 'Switch to the current active version of LUCI UI.';

  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const handleOpenMenu = (event: MouseEvent<HTMLElement>) => {
    setMenuAnchorEl(event.currentTarget);
  };
  const handleCloseMenu = () => {
    setMenuAnchorEl(null);
  };

  return (
    <>
      <IconButton
        onClick={handleOpenMenu}
        color="inherit"
        role="button"
        aria-label="Open menu"
        title="Open menu"
      >
        <MoreVert />
      </IconButton>
      <Menu
        sx={{ mt: 4 }}
        anchorEl={menuAnchorEl}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        keepMounted
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        open={Boolean(menuAnchorEl)}
        onClose={handleCloseMenu}
      >
        <MenuItem title={switchVersionTitle} onClick={() => switchVersion()}>
          <ListItemIcon>
            <VersionControlIcon />
          </ListItemIcon>
          <ListItemText>Switch to {isNewUI ? 'old' : 'new'} UI</ListItemText>
        </MenuItem>
      </Menu>
    </>
  );
}
