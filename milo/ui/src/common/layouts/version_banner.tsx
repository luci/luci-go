// Copyright 2025 The LUCI Authors.
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

import { Alert, AlertTitle, Box, Button, Link, styled } from '@mui/material';

import {
  ROLLBACK_DURATION_WEEK,
  VersionControlIcon,
  useSwitchVersion,
} from '@/common/components/version_control';
import { genFeedbackUrl } from '@/common/tools/utils';

const Container = styled(Box)`
  display: flex;
  background-color: rgb(255, 244, 229);
`;

const LeftGroupContainer = styled(Box)`
  position: sticky;
  // Stick to left in case the page grows wider than 100vw.
  left: calc(var(--accumulated-left) + 16px);
`;

const RightGroupContainer = styled(Box)`
  position: sticky;
  // Stick to right in case the page grows wider than 100vw.
  right: calc(var(--accumulated-right) + 17px);
`;

/**
 * Show users a banner when they are on the old release of LUCI UI.
 */
export function VersionBanner() {
  const switchVersion = useSwitchVersion();

  if (UI_VERSION_TYPE !== 'old-ui') {
    return <></>;
  }

  return (
    <Container aria-label="Old UI Version Banner">
      <LeftGroupContainer>
        <Alert severity="warning" sx={{ padding: 0 }} action={<></>}>
          <AlertTitle>
            <strong>
              Please{' '}
              <Link href={genFeedbackUrl()} target="_blank" rel="noopener">
                report
              </Link>{' '}
              any bug in LUCI UI.
            </strong>{' '}
          </AlertTitle>
          Any bug in the current active version of LUCI UI (
          <Button
            endIcon={<VersionControlIcon />}
            size="small"
            sx={{
              padding: 0,
              marginRight: '4px',
              '& > .MuiButton-endIcon': { marginLeft: '2px' },
            }}
            onClick={() => switchVersion()}
          >
            Switch Over
          </Button>
          ) will appear here in ≤ {ROLLBACK_DURATION_WEEK} weeks if left
          unfixed.
        </Alert>
      </LeftGroupContainer>
      <Box sx={{ flexGrow: 1 }}></Box>
      <RightGroupContainer></RightGroupContainer>
    </Container>
  );
}
