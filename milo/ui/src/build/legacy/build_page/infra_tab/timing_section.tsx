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

import styled from '@emotion/styled';
import { Info } from '@mui/icons-material';
import { IconProps } from '@mui/material';
import { useMemo } from 'react';

import { getTimingInfo } from '@/build/tools/build_utils';
import { DurationBadge } from '@/common/components/duration_badge';
import { Timestamp } from '@/common/components/timestamp';
import { displayDuration } from '@/common/tools/time_utils';

import { useBuild } from '../context';

const InlineInfo = styled(Info)<IconProps>({
  verticalAlign: 'bottom',
});

export function TimingSection() {
  const build = useBuild();

  const info = useMemo(() => build && getTimingInfo(build), [build]);

  if (!build || !info) {
    return <></>;
  }

  const {
    createTime,
    startTime,
    endTime,
    pendingDuration,
    schedulingTimeout,
    exceededSchedulingTimeout,
    executionDuration,
    executionTimeout,
    exceededExecutionTimeout,
  } = info;

  return (
    <>
      <h3>Timing</h3>
      <table
        css={{
          '& td:nth-of-type(2)': {
            clear: 'both',
            overflowWrap: 'anywhere',
          },
        }}
      >
        <tbody>
          <tr>
            <td>Created:</td>
            <td>
              <Timestamp datetime={createTime} />
            </td>
          </tr>
          <tr>
            <td>Started:</td>
            <td>{startTime ? <Timestamp datetime={startTime} /> : 'N/A'}</td>
          </tr>
          <tr>
            <td>Ended:</td>
            <td>{endTime ? <Timestamp datetime={endTime} /> : 'N/A'}</td>
          </tr>
          <tr>
            <td>Pending:</td>
            <td>
              <DurationBadge
                duration={pendingDuration}
                from={createTime}
                to={startTime}
              />
              {exceededSchedulingTimeout ? (
                <span className="warning">(exceeded timeout)</span>
              ) : (
                ''
              )}
              <span
                title={`Maximum pending duration: ${
                  schedulingTimeout ? displayDuration(schedulingTimeout) : 'N/A'
                }`}
              >
                <InlineInfo fontSize="small" />
              </span>
            </td>
          </tr>
          <tr>
            <td>Execution:</td>
            <td>
              <DurationBadge
                duration={executionDuration}
                from={startTime}
                to={endTime}
              />
              {exceededExecutionTimeout ? (
                <span className="warning">(exceeded timeout)</span>
              ) : (
                ''
              )}
              <span
                title={`Maximum execution duration: ${
                  executionTimeout ? displayDuration(executionTimeout) : 'N/A'
                }`}
              >
                <InlineInfo fontSize="small" />
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </>
  );
}
