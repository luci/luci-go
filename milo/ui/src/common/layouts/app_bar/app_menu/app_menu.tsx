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

import DescriptionIcon from '@mui/icons-material/Description';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import SettingsIcon from '@mui/icons-material/Settings';
import { Divider } from '@mui/material';
import IconButton from '@mui/material/IconButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import { useState, MouseEvent } from 'react';
import { Link } from 'react-router';

import { useSetShowPageConfig } from '@/common/components/page_config_state_provider';
import {
  VersionControlIcon,
  useSwitchVersion,
  ROLLBACK_DURATION_WEEK,
} from '@/common/components/version_control';
import { VersionInfo } from '@/common/components/version_info';
import { ReleaseNotesTooltip } from '@/core/components/release_notes';

const isNewUI = UI_VERSION_TYPE === 'new-ui';

export function AppMenu() {
  const setShowPageConfig = useSetShowPageConfig();
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
      <ReleaseNotesTooltip>
        <IconButton
          onClick={handleOpenMenu}
          color="inherit"
          role="button"
          aria-label="Open menu"
          title="Open menu"
        >
          <MoreVertIcon />
        </IconButton>
      </ReleaseNotesTooltip>
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
        <MenuItem
          component={Link}
          to="/ui/doc/release-notes"
          title="View LUCI UI Release Notes"
          onClick={handleCloseMenu}
        >
          <ListItemIcon>
            <DescriptionIcon />
          </ListItemIcon>
          <ListItemText>What{"'"}s new</ListItemText>
        </MenuItem>
        <MenuItem title={switchVersionTitle} onClick={() => switchVersion()}>
          <ListItemIcon>
            <VersionControlIcon />
          </ListItemIcon>
          <ListItemText>Switch to {isNewUI ? 'old' : 'new'} UI</ListItemText>
        </MenuItem>
        <MenuItem
          onClick={() => setShowPageConfig!(true)}
          disabled={setShowPageConfig === null}
          title="Change settings specific to the page."
        >
          <ListItemIcon>
            <SettingsIcon />
          </ListItemIcon>
          <ListItemText>Page Settings</ListItemText>
        </MenuItem>
        <Divider />
        <VersionInfo />
      </Menu>
    </>
  );
}
