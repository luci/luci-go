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

import { Feedback, MoreVert } from '@mui/icons-material';
import { Box, IconButton, Link, LinkProps, styled } from '@mui/material';

import { genFeedbackUrl } from '../libs/utils';
import { AppMenu } from './app_menu';
import { useAuthState } from './auth_state_provider';
import { SignIn } from './signin';

const NavLink = styled(Link)<LinkProps>(() => ({
  color: 'var(--default-text-color)',
  textDecoration: 'underline',
  cursor: 'pointer',
}));

export interface TopBarProps {
  readonly container?: HTMLElement;
}

export function TopBar({ container }: TopBarProps) {
  const authState = useAuthState();

  return (
    <Box
      sx={{
        boxSizing: 'border-box',
        height: '52px',
        padding: '6px 0',
        display: 'flex',
      }}
    >
      <Box
        sx={{
          flex: '1 1 100%',
          alignItems: 'center',
          verticalAlign: 'center',
          marginLeft: '14px',
          lineHeight: '40px',
        }}
      >
        <NavLink href="/">Home</NavLink> |{' '}
        <NavLink href="/search">Search</NavLink>
      </Box>
      <IconButton onClick={() => window.open(genFeedbackUrl())} size="medium">
        <Feedback />
      </IconButton>
      <AppMenu container={container}>
        <MoreVert />
      </AppMenu>
      <Box
        sx={{
          marginRight: '14px',
          flexShrink: 0,
        }}
      >
        <SignIn
          identity={authState.identity}
          email={authState.email}
          picture={authState.picture}
        />
      </Box>
    </Box>
  );
}
