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

import styled from '@emotion/styled';
import LogoutIcon from '@mui/icons-material/Logout';
import Avatar from '@mui/material/Avatar';
import Button from '@mui/material/Button';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useLocation } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { StyledIconButton } from '@/common/components/gm3_styled_components';
import { getLoginUrl, getLogoutUrl } from '@/common/tools/url_utils';

const StyledLoginButton = styled(Button)({
  color: 'var(--gm3-color-on-surface-variant)',
  textTransform: 'none',
  fontSize: '14px',
  fontWeight: 500,
});

const StyledUserEmail = styled(Typography)({
  color: 'var(--gm3-color-on-surface-variant)',
  fontSize: '14px',
  lineHeight: '28px',
  whiteSpace: 'nowrap',
});

const StyledUserAvatar = styled(Avatar)({
  width: 32,
  height: 32,
});

export interface LoginStatusProps {
  readonly identity?: string;
  readonly email?: string;
  readonly picture?: string;
}

export const LoginStatus = ({ identity, email, picture }: LoginStatusProps) => {
  const location = useLocation();

  if (!identity || identity === ANONYMOUS_IDENTITY) {
    return (
      <StyledLoginButton
        href={getLoginUrl(location.pathname + location.search + location.hash)}
      >
        Login
      </StyledLoginButton>
    );
  }
  return (
    <>
      <StyledUserEmail>{email}</StyledUserEmail>
      <StyledUserAvatar
        sx={{ mx: 1 }}
        aria-label="avatar"
        alt={email || 'User Avatar'}
        src={picture}
      />
      <Tooltip title="Logout">
        <StyledIconButton
          aria-label="Logout button"
          href={getLogoutUrl(
            location.pathname + location.search + location.hash,
          )}
        >
          <LogoutIcon />
        </StyledIconButton>
      </Tooltip>
    </>
  );
};
