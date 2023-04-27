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

// import styled from '@emotion/styled';
import { styled, Tab as MuiTab, TabProps, Tabs as MuiTabs } from '@mui/material';
import { Link } from 'react-router-dom';

/**
 * A styled <Tabs /> component. With reduced height and added bottom border.
 */
export const Tabs = styled(MuiTabs)({
  minHeight: '36px',
  borderBottom: '1px solid var(--divider-color)',
});

/**
 * A tab component. Should be placed in <Tabs />.
 */
export function Tab(params: TabProps<typeof Link>) {
  return (
    <MuiTab
      {...params}
      component={Link}
      sx={{
        color: 'var(--default-text-color)',
        padding: '0 14px',
        minHeight: '36px',
      }}
    />
  );
}
