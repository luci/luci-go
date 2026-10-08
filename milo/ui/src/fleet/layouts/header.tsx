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

import {
  AutoAwesome,
  ExtensionOutlined,
  FeedbackOutlined,
} from '@mui/icons-material';
import HelpOutlineOutlinedIcon from '@mui/icons-material/HelpOutlineOutlined';
import LogoutIcon from '@mui/icons-material/Logout';
import MenuIcon from '@mui/icons-material/Menu';
import {
  Avatar,
  Button,
  Divider,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { MouseEvent } from 'react';
import { Link } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { AvailableFlags } from '@/common/layouts/app_bar/available_flags/available_flags';
import { getLoginUrl, getLogoutUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import fleetConsoleMascot from '@/fleet/assets/pngs/fleet-console-mascot.png';
import { PlatformSelector } from '@/fleet/components/platform_selector';
import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';
import { colors } from '@/fleet/theme/colors';

import { FEEDBACK_BUGANIZER_BUG_ID } from '../constants/feedback';
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
  const { trackEvent } = useFleetAnalytics();

  const handleHeaderClickCapture = (event: MouseEvent<HTMLElement>) => {
    if (!(event.target instanceof Element)) {
      return;
    }
    const control = event.target.closest('button, a');
    if (!control) {
      return;
    }
    const label =
      control.getAttribute('aria-label') ||
      control.textContent?.trim() ||
      control.getAttribute('href');
    if (label) {
      trackEvent('header_button_clicked', {
        componentName: label,
      });
    }
  };

  const openExternalLink = (url: string) => {
    trackEvent('external_link_clicked', {
      componentName: url,
    });
    window.open(url);
  };

  return (
    <header
      onClickCapture={handleHeaderClickCapture}
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
          gap: 12,
        }}
      >
        <IconButton
          size="medium"
          aria-label="menu"
          onClick={() => setSidebarOpen(!sidebarOpen)}
          sx={{
            color: colors.grey[700],
            width: 40,
            height: 40,
            padding: '8px',
            marginLeft: '-8px',
          }}
        >
          <MenuIcon />
        </IconButton>
        <Link to="/ui/fleet/" css={{ display: 'flex', alignItems: 'center' }}>
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
          <Link
            to="/ui/fleet/"
            css={{ color: 'inherit', textDecoration: 'none' }}
          >
            Fleet Console
          </Link>
        </Typography>
        {isInPlatformScope && <PlatformSelector />}
      </div>

      <div
        css={{
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          color: colors.grey[700],
          '& .MuiIconButton-root': {
            color: colors.grey[700],
            width: 40,
            height: 40,
            padding: 8,
          },
        }}
      >
        <Tooltip title="Ask Captain Fin (AI assistant)">
          <IconButton
            onClick={() =>
              openExternalLink('http://goto.google.com/captain-fin')
            }
            aria-label="Ask Captain Fin (AI assistant)"
          >
            <AutoAwesome />
          </IconButton>
        </Tooltip>
        <Tooltip title="Fleet Console documentation">
          <IconButton
            onClick={() =>
              openExternalLink('http://goto.google.com/fleet-console')
            }
            aria-label="Fleet Console documentation"
          >
            <HelpOutlineOutlinedIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="Report a bug">
          <IconButton
            onClick={() =>
              openExternalLink(
                genFeedbackUrl({ bugComponent: FEEDBACK_BUGANIZER_BUG_ID }),
              )
            }
            aria-label="Report a bug"
          >
            <FeedbackOutlined />
          </IconButton>
        </Tooltip>
        <Tooltip title="Request a feature">
          <IconButton
            onClick={() =>
              openExternalLink('http://goto.google.com/fcon-feature')
            }
            aria-label="Request a feature"
          >
            <ExtensionOutlined />
          </IconButton>
        </Tooltip>
        <AvailableFlags />
        <SettingsMenu />
        <Divider
          orientation="vertical"
          flexItem
          sx={{
            height: 24,
            alignSelf: 'center',
            mx: '8px',
            borderColor: colors.grey[300],
          }}
        />
        {!authState.identity || authState.identity === ANONYMOUS_IDENTITY ? (
          <Button
            variant="text"
            sx={{
              color: colors.grey[700],
              px: 1.5,
              py: 0.75,
              margin: 0,
              borderRadius: '9999px',
              textTransform: 'none',
              fontWeight: 500,
            }}
            href={getLoginUrl(
              location.pathname + location.search + location.hash,
            )}
          >
            Sign in
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
          mx: '4px',
        }}
        alt={email}
        src={picture}
        aria-label="avatar"
      />
      <Tooltip title="Sign out">
        <IconButton
          className="logout"
          aria-label="Sign out"
          color="inherit"
          href={getLogoutUrl(
            location.pathname + location.search + location.hash,
          )}
          sx={{
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
