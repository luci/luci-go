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

import BugReportOutlinedIcon from '@mui/icons-material/BugReportOutlined';
import HelpOutlineOutlinedIcon from '@mui/icons-material/HelpOutlineOutlined';
import LogoutIcon from '@mui/icons-material/Logout';
import MenuIcon from '@mui/icons-material/Menu';
import {
  Avatar,
  Button,
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl, getLogoutUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import fleetConsoleMascot from '@/fleet/assets/pngs/fleet-console-mascot.png';
import { colors } from '@/fleet/theme/colors';

export const Header = ({
  sidebarOpen,
  setSidebarOpen,
}: {
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
}) => {
  const authState = useAuthState();

  return (
    <header
      css={{
        display: 'flex',
        justifyContent: 'space-between',
        backgroundColor: colors.white,
        borderBottom: `solid ${colors.grey[300]} 1px`,
        padding: '0 20px',
        zIndex: 1200,
        boxSizing: 'border-box',
        position: 'sticky',
        top: 0,
        left: 0,
        width: '100%',
        height: 64,
      }}
    >
      <div
        css={{
          display: 'flex',
          alignItems: 'center',
          gap: 13,
        }}
      >
        <IconButton
          size="large"
          aria-label="menu"
          onClick={() => setSidebarOpen(!sidebarOpen)}
          sx={{
            color: colors.grey[700],
            padding: 0,
          }}
        >
          <MenuIcon />
        </IconButton>
        <Link
          to="/ui/fleet/labs/devices"
          css={{ display: 'flex', alignItems: 'center' }}
        >
          <img
            alt="logo"
            id="luci-icon"
            src={fleetConsoleMascot}
            css={{
              width: 55,
              padding: 5,
            }}
          />
        </Link>
        <Typography variant="h5" sx={{ color: colors.grey[700] }}>
          Fleet Console
        </Typography>
      </div>

      <div
        css={{
          display: 'flex',
          alignItems: 'center',
          gap: 13,
        }}
      >
        <Tooltip title="Fleet Console, work in progress">
          <HelpOutlineOutlinedIcon sx={{ color: colors.grey[700] }} />
        </Tooltip>
        <IconButton
          onClick={() =>
            window.open(genFeedbackUrl({ bugComponent: '1664178' }))
          }
        >
          <BugReportOutlinedIcon sx={{ color: colors.grey[700] }} />
        </IconButton>
        {!authState.identity || authState.identity === ANONYMOUS_IDENTITY ? (
          <Button
            variant="text"
            sx={{ color: colors.grey[700], padding: 0, marign: 0 }}
            href={getLoginUrl(
              location.pathname + location.search + location.hash,
            )}
          >
            Login
          </Button>
        ) : (
          <LoggedInAvatar email={authState.email} picture={authState.picture} />
        )}
      </div>
    </header>
  );
};

function LoggedInAvatar({
  email,
  picture,
}: {
  email?: string;
  picture?: string;
}) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const navigate = useNavigate();

  return (
    <>
      <Avatar
        sx={{
          cursor: 'pointer',
          width: 32,
          height: 32,
        }}
        alt={email}
        src={picture}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        aria-label="avatar"
      />
      <Menu
        anchorEl={anchorEl}
        open={!!anchorEl}
        onClose={() => setAnchorEl(null)}
        MenuListProps={{
          sx: { padding: 0 },
        }}
      >
        <MenuItem
          sx={{ padding: '0 20px 0 15px ', height: 48 }}
          onClick={() =>
            navigate(
              getLogoutUrl(location.pathname + location.search + location.hash),
            )
          }
        >
          <LogoutIcon sx={{ marginRight: '10px' }} />
          <Typography>Logout</Typography>
        </MenuItem>
      </Menu>
    </>
  );
}
