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

import LogoutIcon from '@mui/icons-material/Logout';
import { IconButton } from '@mui/material';
import Avatar from '@mui/material/Avatar';
import Button from '@mui/material/Button';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useLocation } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { getLoginUrl, getLogoutUrl } from '@/common/tools/url_utils';

export interface SignInProps {
  readonly identity?: string;
  readonly email?: string;
  readonly picture?: string;
}

export const SignIn = ({ identity, email, picture }: SignInProps) => {
  const location = useLocation();

  if (!identity || identity === ANONYMOUS_IDENTITY) {
    return (
      <Button
        variant="text"
        sx={{ color: 'white' }}
        href={getLoginUrl(location.pathname + location.search + location.hash)}
      >
        Login
      </Button>
    );
  }

  return (
    <>
      <Typography sx={{ ml: 1 }}>{email}</Typography>
      <Avatar aria-label="avatar" sx={{ mx: 1 }} alt={email} src={picture} />
      <Tooltip title="Logout">
        <IconButton
          aria-label="Logout button"
          color="inherit"
          href={getLogoutUrl(
            location.pathname + location.search + location.hash,
          )}
        >
          <LogoutIcon />
        </IconButton>
      </Tooltip>
    </>
  );
};
