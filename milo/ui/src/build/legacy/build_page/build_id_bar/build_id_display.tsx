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

import { Box, Link, SxProps, Theme, styled } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';

import { getBuilderURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

const Container = styled(Box)`
  flex: 0 auto;
  font-size: 20px;
  font-weight: bold;
`;

function Divider() {
  return <span css={{ opacity: 0.2 }}> / </span>;
}

export interface BuildIdDisplayProps {
  readonly builderId: BuilderID;
  readonly buildNumOrId: string;
  readonly sx?: SxProps<Theme>;
}

export function BuildIdDisplay({
  builderId,
  buildNumOrId,
  sx,
}: BuildIdDisplayProps) {
  return (
    <Container sx={sx}>
      <span css={{ opacity: 0.4 }}>Build: </span>
      <span>{builderId.bucket}</span>
      <Divider />
      <Link component={RouterLink} to={getBuilderURLPath(builderId)}>
        {builderId.builder}
      </Link>
      <Divider />
      <span>{buildNumOrId}</span>
    </Container>
  );
}
