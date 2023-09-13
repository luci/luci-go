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

import MuiAppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Toolbar from '@mui/material/Toolbar';

import { useAuthState } from '@/common/components/auth_state_provider';

import { AppDetails } from './app_details';
import { AppMenu } from './app_menu';
import { FeedbackButton } from './feedback';
import { SignIn } from './sign_in';

interface Props {
  open: boolean;
  handleSidebarChanged: (isOpen: boolean) => void;
}

export const AppBar = ({ open, handleSidebarChanged }: Props) => {
  const authState = useAuthState();

  return (
    <MuiAppBar
      position="fixed"
      sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}
    >
      <Toolbar variant="dense">
        <AppDetails open={open} handleSidebarChanged={handleSidebarChanged} />
        <Box sx={{ flexGrow: 1 }}></Box>
        <FeedbackButton />
        <AppMenu />
        <SignIn
          identity={authState.identity}
          email={authState.email}
          picture={authState.picture}
        />
      </Toolbar>
    </MuiAppBar>
  );
};
