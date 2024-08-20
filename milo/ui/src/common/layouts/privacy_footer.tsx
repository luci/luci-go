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

import { Box, Link, styled } from '@mui/material';

const Container = styled(Box)`
  display: flex;
  position: relative;
  background-color: var(--block-background-color);
  border-top: solid 1px var(--divider-color);
`;

const RightLinksGroup = styled(Box)`
  display: flex;
  align-items: center;
  position: sticky;
  right: calc(var(--accumulated-right) + 3px);
  justify-content: end;
  gap: 15px;
  line-height: 15px;
  font-size: 14px;
  padding: 15px 15px;
  margin-top: auto;
`;

const MutedLink = styled(Link)`
  text-decoration: none;
  color: rgb(31, 31, 31);
`;

export function PrivacyFooter() {
  return (
    <Container>
      <Box sx={{ flexGrow: 1 }} />
      {/* PrivacyFooter can grow wider than the viewport. Put items in a sticky
       ** group to ensure they always stay at the same place.
       */}
      <RightLinksGroup>
        <MutedLink
          href="https://policies.google.com/privacy"
          target="_blank"
          rel="noreferrer"
        >
          Privacy
        </MutedLink>
        <MutedLink
          href="https://policies.google.com/terms"
          target="_blank"
          rel="noreferrer"
        >
          Terms
        </MutedLink>
      </RightLinksGroup>
    </Container>
  );
}
