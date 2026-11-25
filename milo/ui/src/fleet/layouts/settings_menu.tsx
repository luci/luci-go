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
import BrightnessAutoOutlined from '@mui/icons-material/BrightnessAutoOutlined';
import CheckIcon from '@mui/icons-material/Check';
import DarkModeOutlined from '@mui/icons-material/DarkModeOutlined';
import DescriptionIcon from '@mui/icons-material/Description';
import LaunchIcon from '@mui/icons-material/Launch';
import LightModeOutlined from '@mui/icons-material/LightModeOutlined';
import {
  Divider,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Typography,
} from '@mui/material';
import { MouseEvent, ReactNode, useState } from 'react';
import { Link } from 'react-router';

import {
  VersionControlIcon,
  useSwitchVersion,
  ROLLBACK_DURATION_WEEK,
} from '@/common/components/version_control';
import { VersionInfo } from '@/common/components/version_info';
import {
  FleetThemeChoice,
  useFleetTheme,
} from '@/fleet/theme/fleet_theme_context';

const isNewUI = UI_VERSION_TYPE === 'new-ui';

const THEME_OPTIONS: {
  id: FleetThemeChoice;
  label: string;
  title: string;
  icon: ReactNode;
}[] = [
  {
    id: 'light',
    label: 'Light',
    title: 'Light theme',
    icon: <LightModeOutlined fontSize="small" />,
  },
  {
    id: 'dark',
    label: 'Dark',
    title: 'Dark theme',
    icon: <DarkModeOutlined fontSize="small" />,
  },
  {
    id: 'auto',
    label: 'Auto (system)',
    title: 'Follow your system setting',
    icon: <BrightnessAutoOutlined fontSize="small" />,
  },
];

export function SettingsMenu() {
  const switchVersion = useSwitchVersion();
  const fleetTheme = useFleetTheme();
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
        <MenuItem
          component={Link}
          to="http://goto.google.com/fleet-console-newsletter"
          title="View Fleet Console newsletter"
          onClick={handleCloseMenu}
          target="_blank"
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
        <Divider />
        <MenuItem
          component="a"
          href="https://luci-scheduler.appspot.com/jobs/infra-internal/luci-ui-promoter"
          target="_blank"
          rel="noopener"
          onClick={handleCloseMenu}
          sx={{ color: 'inherit', textDecoration: 'none' }}
        >
          <ListItemIcon>
            <LaunchIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Frontend Promoter</ListItemText>
        </MenuItem>
        <MenuItem
          component="a"
          href="https://luci-scheduler.appspot.com/jobs/infra-internal/fleet-console-backend-promoter"
          target="_blank"
          rel="noopener"
          onClick={handleCloseMenu}
          sx={{ color: 'inherit', textDecoration: 'none' }}
        >
          <ListItemIcon>
            <LaunchIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Backend Promoter</ListItemText>
        </MenuItem>
        {fleetTheme && <Divider />}
        {fleetTheme && (
          <Typography
            variant="caption"
            color="text.secondary"
            component="li"
            role="presentation"
            sx={{ px: 2, pt: 0.5, pb: 0.25, display: 'block' }}
          >
            Theme
          </Typography>
        )}
        {fleetTheme &&
          THEME_OPTIONS.map(({ id, label, title, icon }) => {
            const selected = fleetTheme.choice === id;
            return (
              <MenuItem
                key={id}
                role="menuitemradio"
                aria-checked={selected}
                selected={selected}
                title={title}
                onClick={() => {
                  fleetTheme.setChoice(id, 'settings_menu');
                  handleCloseMenu();
                }}
              >
                <ListItemIcon>{icon}</ListItemIcon>
                <ListItemText>{label}</ListItemText>
                {selected && (
                  <CheckIcon fontSize="small" sx={{ ml: 2 }} aria-hidden />
                )}
              </MenuItem>
            );
          })}
        <Divider />
        <VersionInfo />
      </Menu>
    </>
  );
}
