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
import { DateTime } from 'luxon';

import { Timestamp } from '@/common/components/timestamp';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { testStatusToJSON } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { ResultStatusIcon } from '@/test_verdict/components/result_status_icon';
import { OutputArtifactMatchingContent } from '@/test_verdict/types';

const Container = styled(Box)`
  display: flex;
  gap: 20px;
  border-bottom: 1px solid #dadce0;
  padding: 2px 20px;
  font-size: 15px;
  line-height: 1.5;
  letter-spacing: normal;
`;

export interface LogSnippetRowProps {
  readonly artifact: OutputArtifactMatchingContent;
}

export function LogSnippetRow({ artifact }: LogSnippetRowProps) {
  const { name, partitionTime, testStatus, beforeMatch, match, afterMatch } =
    artifact;
  return (
    <Container>
      <Box>
        <Timestamp
          datetime={DateTime.fromISO(partitionTime!)}
          format="MMM dd, HH:mm"
        />
        <Box
          sx={{
            alignItems: 'center',
            display: 'flex',
          }}
        >
          <ResultStatusIcon status={testStatus} sx={{ fontSize: '22px' }} />
          {testStatusToJSON(testStatus)}
        </Box>
      </Box>
      <Link
        href={getRawArtifactURLPath(name)}
        underline="none"
        color="inherit"
        target="_blank"
        rel="noopenner"
      >
        <Box
          sx={{
            whiteSpace: 'pre-line',
          }}
        >
          {beforeMatch}
          <span css={{ backgroundColor: '#ceead6', fontWeight: '700' }}>
            {match}
          </span>
          {afterMatch}
        </Box>
      </Link>
    </Container>
  );
}
