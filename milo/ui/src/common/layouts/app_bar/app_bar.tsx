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

import { Box, styled } from '@mui/material';
import MuiAppBar from '@mui/material/AppBar';

import { useAuthState } from '@/common/components/auth_state_provider';
import { StyledToolbar } from '@/common/components/gm3_styled_components';
import { LoginStatus } from '@/common/components/user';

import { AppDetails } from './app_details';
import { AppMenu } from './app_menu';
import { AvailableFlags } from './available_flags';
import { FeedbackButton } from './feedback';

const ItemGroup = styled(Box)`
  display: flex;
  align-items: center;
  position: sticky;
`;

const StyledMuiAppBar = styled(MuiAppBar)(() => ({
  background: 'var(--gm3-color-surface)', // Changed from #FFFFFF
  color: 'var(--gm3-color-on-surface-variant)',
  height: '64px',
  boxShadow: 'none',
  borderBottom: '1px solid var(--gm3-color-outline-variant)',
}));

interface Props {
  open: boolean;
  handleSidebarChanged: (isOpen: boolean) => void;
}

export const AppBar = ({ open, handleSidebarChanged }: Props) => {
  const authState = useAuthState();

  return (
    <StyledMuiAppBar
      // Use static so the element occupies space so we can calculate its
      // height by measuring its parent. The height will be used to calculate
      // how much offset the next sticky item needs to avoid overlapping.
      //
      // The sticky behavior of the App bar is implemented by the parent.
      position="static"
    >
      <StyledToolbar variant="dense">
        {/* AppBar can grow wider than the viewport. Divide the items into left
         ** group and right group and make them sticky to ensure they always
         ** stay at the same place.
         */}
        <ItemGroup
          sx={{ left: 'calc(var(--accumulated-left) + 25px)', gap: 2 }}
        >
          <AppDetails open={open} handleSidebarChanged={handleSidebarChanged} />
        </ItemGroup>
        <Box sx={{ flexGrow: 1 }}></Box>
        <ItemGroup
          sx={{ right: 'calc(var(--accumulated-right) + 30px)', gap: 1 }}
        >
          <AvailableFlags />
          <FeedbackButton />
          <AppMenu />
          <LoginStatus
            identity={authState.identity}
            email={authState.email}
            picture={authState.picture}
          />
        </ItemGroup>
      </StyledToolbar>
    </StyledMuiAppBar>
  );
};
