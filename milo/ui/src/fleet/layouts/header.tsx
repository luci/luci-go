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
import MenuIcon from '@mui/icons-material/Menu';
import {
  Avatar,
  Box,
  Button,
  colors,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { Link } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import fleetConsoleMascot from '@/fleet/assets/pngs/fleet-console-mascot.png';

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
        flexDirection: 'row',
        gap: 10,
        alignItems: 'center',
        backgroundColor: 'white',
        borderBottom: `solid ${colors.grey[100]} 2px`,
        padding: '0 20px',
        zIndex: 1200,
        boxSizing: 'border-box',
        position: 'sticky',
        top: 0,
        left: 0,
        width: '100%',
      }}
    >
      <IconButton
        size="large"
        edge="start"
        color="inherit"
        aria-label="menu"
        onClick={() => setSidebarOpen(!sidebarOpen)}
        sx={{
          color: colors.grey[600],
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
            width: 35,
            padding: 5,
          }}
        />
      </Link>

      <Typography sx={{ color: colors.grey[600] }} variant="subtitle1">
        Fleet Console
      </Typography>

      <Box sx={{ flexGrow: 1 }} />

      <Tooltip title="Fleet Console, work in progress">
        <HelpOutlineOutlinedIcon sx={{ color: colors.grey[600] }} />
      </Tooltip>
      <IconButton
        onClick={() => window.open(genFeedbackUrl({ bugComponent: '1664178' }))}
      >
        <BugReportOutlinedIcon sx={{ color: colors.grey[600] }} />
      </IconButton>
      {!authState.identity || authState.identity === ANONYMOUS_IDENTITY ? (
        <Button
          variant="text"
          sx={{ color: colors.grey[600] }}
          href={getLoginUrl(
            location.pathname + location.search + location.hash,
          )}
        >
          Login
        </Button>
      ) : (
        <Avatar
          aria-label="avatar"
          sx={{ width: 30, height: 30 }}
          alt={authState.email}
          src={authState.picture}
        />
      )}
    </header>
  );
};
