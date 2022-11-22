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
import { BroadcastChannel } from 'broadcast-channel';

import { ANONYMOUS_IDENTITY } from '../services/milo_internal';

/**
 * Signs in/out the user.
 *
 * @param signIn pass true to sign in, false to sign out.
 */
export function changeUserState(signIn: boolean) {
  const channelId = 'auth-channel-' + Math.random();
  const redirectUrl = `/ui/auth-callback/${channelId}`;
  const target = window.open(
    `/auth/openid/${signIn ? 'login' : 'logout'}?${new URLSearchParams({ r: redirectUrl })}`,
    '_blank'
  );
  if (!target) {
    return;
  }

  const channel = new BroadcastChannel(channelId);

  // Close the channel in 1hr to prevent memory leak in case the target never
  // sent the message.
  const timeout = window.setTimeout(() => channel.close(), 3600000);

  channel.addEventListener('message', () => {
    window.clearTimeout(timeout);
    channel.close();
    target.close();
  });
}

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
  if (!identity || identity === ANONYMOUS_IDENTITY) {
    return (
      <Container>
        <ActionLink onClick={() => changeUserState(true)}>Login</ActionLink>
      </Container>
    );
  }

  return (
    <Container>
      {picture && (
        <img
          src={picture}
          css={{ margin: '2px 3px', height: '36px', width: '36px', borderRadius: '6px', overflow: 'hidden' }}
        />
      )}
      <Box
        sx={{
          display: 'inline-block',
          height: '40px',
          verticalAlign: 'top',
        }}
      >
        {email} | <ActionLink onClick={() => changeUserState(false)}>Logout</ActionLink>
      </Box>
    </Container>
  );
}
