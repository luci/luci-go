// Copyright 2023 The LUCI Authors.
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

import Avatar from '@mui/material/Avatar';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import Typography from '@mui/material/Typography';

import { useGlobals } from '../context/globals';

// Shows Login/Logout buttons and the account info.
//
// If login sessions are not supported, just renders nothing.
export const LoginState = () => {
  const { tokenClient } = useGlobals();

  // When using stateless auth, there's no login/logout at all.
  if (tokenClient.sessionState == 'stateless') {
    return <></>;
  }

  // If logged out, show the login button. Note that it does regular browser
  // navigation resulting in the page reload: no need to hook up anything to it.
  if (tokenClient.sessionState == 'loggedout') {
    return <Button color="inherit" href={tokenClient.loginUrl}>Login</Button>;
  }

  // Otherwise show the account state and the logout button.
  const account = tokenClient.sessionState;
  return (
    <>
      {
        account.picture && <Avatar alt={account.email} src={account.picture} />
      }
      <Typography sx={{ pl: 1 }}>{account.email}</Typography>
      <Divider
        orientation='vertical'
        variant='middle'
        flexItem
        sx={{ pl: 1, borderColor: '#ffffff40' }}
      />
      <Button color="inherit" href={tokenClient.logoutUrl}>Logout</Button>
    </>
  );
};
