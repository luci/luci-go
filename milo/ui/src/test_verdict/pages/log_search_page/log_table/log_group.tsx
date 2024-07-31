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

import {
  InsertDriveFile,
  ArrowForwardIos,
  AspectRatio,
} from '@mui/icons-material';
import { Box, Link, Button, styled } from '@mui/material';

import { getTestHistoryURLPath } from '@/common/tools/url_utils';
import { OutputQueryTestVariantArtifactGroupsResponse_MatchGroup } from '@/test_verdict/types';

import { useLogGroupListDispatch } from '../contexts';
import { LogSnippetRow } from '../log_snippet_row';
import { VariantLine } from '../variant_line';

const LogGroupHeaderDiv = styled(Box)`
  background: #e8f0fe;
  padding: 2px 5px;
  display: flex;
  align-items: center;
  font-size: 14px;
  line-height: 1.5;
  letter-spacing: normal;
  gap: 5px;
  flex-wrap: wrap;
`;

const ExpandableRowDiv = styled(Box)`
  text-align: center;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
`;

interface LogGroupProps {
  readonly project: string;
  readonly group: OutputQueryTestVariantArtifactGroupsResponse_MatchGroup;
}

export function LogGroup({ project, group }: LogGroupProps) {
  const { testId, variant, variantHash, artifactId, artifacts, matchingCount } =
    group;
  const dispatch = useLogGroupListDispatch();

  return (
    <>
      <LogGroupHeaderDiv>
        <InsertDriveFile color="action" sx={{ fontSize: '17px' }} />
        <Link
          href={getTestHistoryURLPath(project, testId)}
          color="inherit"
          underline="hover"
          target="_blank"
          rel="noopenner"
        >
          {testId}
        </Link>
        <ArrowForwardIos sx={{ fontSize: '14px' }} />
        <Box>{variant && <VariantLine variant={variant} />}</Box>
        <ArrowForwardIos sx={{ fontSize: '14px' }} />
        {artifactId}
      </LogGroupHeaderDiv>
      {artifacts.map((a) => (
        <LogSnippetRow artifact={a} key={a.name} />
      ))}
      {matchingCount - artifacts.length > 0 && (
        <Button
          onClick={() =>
            dispatch({
              type: 'showLogGroupList',
              logGroupIdentifer: {
                testID: testId,
                variantHash,
                variant,
                artifactID: artifactId,
              },
            })
          }
          sx={{ width: '100%', textTransform: 'none', fontSize: 'inherit' }}
        >
          <ExpandableRowDiv>
            <AspectRatio sx={{ fontSize: '15px' }} />
            Show {matchingCount - artifacts.length} more matching log for this
            test varaint artifact
          </ExpandableRowDiv>
        </Button>
      )}
    </>
  );
}
