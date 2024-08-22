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

import { Box, Skeleton, styled } from '@mui/material';
import { DateTime } from 'luxon';

import { BuildActionButton } from '@/build/components/build_action_button';
import { BuildBugButton } from '@/build/components/build_bug_button';
import { BuildStatusIcon } from '@/build/components/build_status_icon';
import { DurationBadge } from '@/common/components/duration_badge';
import { Timestamp } from '@/common/components/timestamp';
import { CommitLink } from '@/gitiles/components/commit_link';
import { CompactClList } from '@/gitiles/components/compact_cl_list';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';

import { useBuild } from '../hooks';

import { AncestorBuildPath } from './ancestor_build_path';
import { BuildIdDisplay } from './build_id_display';

const Container = styled(Box)`
  display: flex;
  padding: 6px 16px;
`;

const LeftGroupContainer = styled(Box)`
  position: sticky;
  // Stick to left in case the page grows wider than 100vw.
  left: calc(var(--accumulated-left) + 16px);
  display: grid;
  grid-template-rows: auto auto auto;
  grid-template-columns: auto auto;
  grid-template-areas:
    'none ancestors'
    'icon id'
    'icon info';
  gap: 4px;
`;

const RightGroupContainer = styled(Box)`
  position: sticky;
  // Stick to right in case the page grows wider than 100vw.
  right: calc(var(--accumulated-right) + 17px);
  display: grid;
  grid-template-rows: 1fr auto;
`;

const Label = styled('span')`
  &:not(:first-of-type) {
    margin-left: 10px;
  }
  margin-right: 4px;
  opacity: 0.4;
`;

export interface BuildIdBarProps {
  readonly builderId: BuilderID;
  readonly buildNumOrId: string;
}

export function BuildIdBar({ builderId, buildNumOrId }: BuildIdBarProps) {
  const build = useBuild();

  const realBuilderId = build?.builder || builderId;
  const realBuildNumOrId = (
    build?.number ||
    (build?.id && `b${build.id}`) ||
    buildNumOrId
  ).toString();

  const commit = build?.output?.gitilesCommit || build?.input?.gitilesCommit;
  const changes = build?.input?.gerritChanges || [];

  return (
    <Container>
      <LeftGroupContainer>
        {build?.ancestorIds.length ? (
          <AncestorBuildPath build={build} sx={{ gridArea: 'ancestors' }} />
        ) : (
          <></>
        )}
        <Box sx={{ gridArea: 'icon', position: 'relative' }}>
          <Box
            sx={{
              position: 'relative',
              top: '50%',
              transform: 'translateY(-50%)',
            }}
          >
            {build ? (
              <BuildStatusIcon
                status={build?.status}
                sx={{ fontSize: '50px' }}
              />
            ) : (
              <Skeleton variant="circular" width={50} height={50} />
            )}
          </Box>
        </Box>
        <BuildIdDisplay
          builderId={realBuilderId}
          buildNumOrId={realBuildNumOrId}
          builderDescription={build?.builderInfo?.description}
          sx={{ gridArea: 'id' }}
        />
        <Box sx={{ gridArea: 'info' }}>
          {build ? (
            <>
              {changes.length > 0 && (
                <>
                  <Label>CL:</Label>
                  <CompactClList changes={changes} />
                </>
              )}
              {commit && (
                <>
                  <Label>Commit:</Label>
                  <CommitLink commit={commit} />
                </>
              )}
              <Label>Created at:</Label>
              <Timestamp
                datetime={build && DateTime.fromISO(build.createTime)}
                format="MMM dd HH:mm"
              />
              <Label>Duration:</Label>
              <DurationBadge
                durationLabel="Execution Duration"
                fromLabel="Start Time"
                from={
                  build.startTime ? DateTime.fromISO(build.startTime) : null
                }
                toLabel="End Time"
                to={build.endTime ? DateTime.fromISO(build.endTime) : null}
              />
            </>
          ) : (
            <Skeleton />
          )}
        </Box>
      </LeftGroupContainer>
      <Box sx={{ flexGrow: 1 }}></Box>
      <RightGroupContainer>
        <Box sx={{ gridRow: '2' }}>
          {build && (
            <BuildActionButton
              build={build}
              variant="contained"
              size="small"
              sx={{ margin: '5px' }}
            />
          )}
          <BuildBugButton
            project={builderId.project}
            build={build}
            variant="contained"
            size="small"
            sx={{ margin: '5px' }}
          />
        </Box>
      </RightGroupContainer>
    </Container>
  );
}
