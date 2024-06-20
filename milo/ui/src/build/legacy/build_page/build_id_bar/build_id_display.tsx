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

import { Box, Button, Link, SxProps, Theme, styled } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useLocalStorage } from 'react-use';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { getBuilderURLPath, getProjectURLPath } from '@/common/tools/url_utils';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

// TODO: remove and the project link and the tooltip completely ~2 weeks after
// it's released.
const HIDE_PROJECT_LINK_TOOLTIP_KEY = '2024-06-13-hide-project-link-tooltip';

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
  const [hideProjectLinkTooltip = false, setHideProjectLinkTooltip] =
    useLocalStorage<boolean>(HIDE_PROJECT_LINK_TOOLTIP_KEY);

  return (
    <Container sx={sx}>
      <span css={{ opacity: 0.4 }}>Build: </span>
      <HtmlTooltip
        arrow
        title={
          !hideProjectLinkTooltip && (
            <>
              <p>This link will be removed in a future release.</p>
              <p>The project page can be access from the top bar.</p>
              <Box
                sx={{
                  display: 'grid',
                  gridTemplateColumns: '1fr auto',
                  marginTop: '20px',
                }}
              >
                <Box />
                <Button
                  size="small"
                  onClick={() => setHideProjectLinkTooltip(true)}
                >
                  {"Don't show again"}
                </Button>
              </Box>
            </>
          )
        }
      >
        <Link component={RouterLink} to={getProjectURLPath(builderId.project)}>
          {builderId.project}
        </Link>
      </HtmlTooltip>
      <Divider />
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
