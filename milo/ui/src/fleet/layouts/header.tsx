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

import { Feedback } from '@mui/icons-material';
import HelpOutlineOutlinedIcon from '@mui/icons-material/HelpOutlineOutlined';
import LogoutIcon from '@mui/icons-material/Logout';
import MenuIcon from '@mui/icons-material/Menu';
import { Avatar, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { Link } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl, getLogoutUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import fleetConsoleMascot from '@/fleet/assets/pngs/fleet-console-mascot.png';
import { PlatformSelector } from '@/fleet/components/platform_selector';
import { colors } from '@/fleet/theme/colors';

import { FEEDBACK_BUGANIZER_BUG_ID } from '../constants/feedback';
import { generateDeviceListURL, CHROMEOS_PLATFORM } from '../constants/paths';
import { useIsInPlatformScope } from '../hooks/usePlatform';

import { SettingsMenu } from './settings_menu';

export const Header = ({
  sidebarOpen,
  setSidebarOpen,
}: {
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
}) => {
  const authState = useAuthState();
  const isInPlatformScope = useIsInPlatformScope();

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
          to={generateDeviceListURL(CHROMEOS_PLATFORM)}
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
        {isInPlatformScope && <PlatformSelector />}
      </div>

      <div
        css={{
          display: 'flex',
          alignItems: 'center',
          gap: 13,
        }}
      >
        <IconButton onClick={() => window.open('http://go/fleet-console')}>
          <Tooltip title="Fleet Console, work in progress">
            <HelpOutlineOutlinedIcon sx={{ color: colors.grey[700] }} />
          </Tooltip>
        </IconButton>
        <IconButton
          onClick={() =>
            window.open(
              genFeedbackUrl({ bugComponent: FEEDBACK_BUGANIZER_BUG_ID }),
            )
          }
        >
          <Tooltip title="File a bug">
            <Feedback sx={{ color: colors.grey[700] }} />
          </Tooltip>
        </IconButton>
        <SettingsMenu />
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
  return (
    <>
      <Avatar
        className="avatar"
        sx={{
          width: 32,
          height: 32,
        }}
        alt={email}
        src={picture}
        aria-label="avatar"
      />
      <Tooltip title="Logout">
        <IconButton
          className="logout"
          aria-label="Logout button"
          color="inherit"
          href={getLogoutUrl(
            location.pathname + location.search + location.hash,
          )}
          sx={{
            color: colors.grey[700],
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
          }}
        >
          <LogoutIcon
            sx={{
              width: 20,
              height: 20,
            }}
          />
        </IconButton>
      </Tooltip>
    </>
  );
}
