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

import { Box, BoxProps, Link, LinkProps, styled } from '@mui/material';
import { useLocation } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '../libs/auth_state';
import { getLoginUrl, getLogoutUrl } from '../libs/url_utils';

const Container = styled(Box)<BoxProps>(() => ({
  display: 'inline-block',
  height: '40px',
  lineHeight: '40px',
}));

const ActionLink = styled(Link)<LinkProps>(() => ({
  color: 'var(--default-text-color)',
  textDecoration: 'underline',
  cursor: 'pointer',
}));

export interface SignInProps {
  readonly identity?: string;
  readonly email?: string;
  readonly picture?: string;
}

export function SignIn({ identity, email, picture }: SignInProps) {
  const location = useLocation();

  if (!identity || identity === ANONYMOUS_IDENTITY) {
    return (
      <Container>
        <ActionLink
          href={getLoginUrl(
            location.pathname + location.search + location.hash
          )}
        >
          Login
        </ActionLink>
      </Container>
    );
  }

  return (
    <Container>
      {picture && (
        <img
          src={picture}
          alt="user avatar"
          css={{
            margin: '2px 3px',
            height: '36px',
            width: '36px',
            borderRadius: '6px',
            overflow: 'hidden',
          }}
        />
      )}
      <Box
        sx={{
          display: 'inline-block',
          height: '40px',
          verticalAlign: 'top',
        }}
      >
        {email} |{' '}
        <ActionLink
          href={getLogoutUrl(
            location.pathname + location.search + location.hash
          )}
        >
          Logout
        </ActionLink>
      </Box>
    </Container>
  );
}
