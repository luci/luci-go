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

import { InsertDriveFile, AspectRatio } from '@mui/icons-material';
import { Button, styled, Box } from '@mui/material';
import { ReactNode } from 'react';

import {
  OutputQueryTestVariantArtifactGroupsResponse_MatchGroup,
  OutputQueryInvocationVariantArtifactGroupsResponse_MatchGroup,
} from '@/test_verdict/types';

import { useLogGroupListDispatch, Action } from '../contexts';
import { LogSnippetRow } from '../log_snippet_row';

const LogGroupHeaderDiv = styled(Box)`
  background: #e8f0fe;
  padding: 2px 5px;
  display: flex;
  align-items: center;
  font-size: 15px;
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

export interface LogGroupProps {
  readonly groupHeader: ReactNode;
  readonly group:
    | OutputQueryTestVariantArtifactGroupsResponse_MatchGroup
    | OutputQueryInvocationVariantArtifactGroupsResponse_MatchGroup;
  readonly dialogAction: Action;
}

export function LogGroup({ groupHeader, group, dialogAction }: LogGroupProps) {
  const { artifacts, matchingCount } = group;
  const dispatch = useLogGroupListDispatch();

  return (
    <>
      <LogGroupHeaderDiv>
        <InsertDriveFile color="action" sx={{ fontSize: '17px' }} />
        {groupHeader}
      </LogGroupHeaderDiv>
      {artifacts.map((a) => (
        <LogSnippetRow artifact={a} key={a.name} />
      ))}
      {matchingCount - artifacts.length > 0 && (
        <Button
          onClick={() => dispatch(dialogAction)}
          sx={{ width: '100%', textTransform: 'none', fontSize: 'inherit' }}
        >
          <ExpandableRowDiv>
            <AspectRatio sx={{ fontSize: '15px' }} />
            Show {matchingCount - artifacts.length} more matching log for this
            group
          </ExpandableRowDiv>
        </Button>
      )}
    </>
  );
}
