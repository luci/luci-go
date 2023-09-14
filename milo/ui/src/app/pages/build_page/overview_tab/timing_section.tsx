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
import { observer } from 'mobx-react-lite';

import { Timestamp } from '@/common/components/timestamp';
import { useStore } from '@/common/store';
import { displayDuration } from '@/common/tools/time_utils';

const InlineInfo = styled(Info)<IconProps>({
  verticalAlign: 'bottom',
});

export const TimingSection = observer(() => {
  const store = useStore();

  const build = store.buildPage.build;
  if (!build) {
    return <></>;
  }
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
              <Timestamp datetime={build.createTime} />
            </td>
          </tr>
          <tr>
            <td>Started:</td>
            <td>
              {build.startTime ? (
                <Timestamp datetime={build.startTime} />
              ) : (
                'N/A'
              )}
            </td>
          </tr>
          <tr>
            <td>Ended:</td>
            <td>
              {build.endTime ? <Timestamp datetime={build.endTime} /> : 'N/A'}
            </td>
          </tr>
          <tr>
            <td>Pending:</td>
            <td>
              {displayDuration(build.pendingDuration)}
              {build.isPending ? '(and counting)' : ''}
              {build.exceededSchedulingTimeout ? (
                <span className="warning">(exceeded timeout)</span>
              ) : (
                ''
              )}
              <span
                title={`Maximum pending duration: ${
                  build.schedulingTimeout
                    ? displayDuration(build.schedulingTimeout)
                    : 'N/A'
                }`}
              >
                <InlineInfo fontSize="small" />
              </span>
            </td>
          </tr>
          <tr>
            <td>Execution:</td>
            <td>
              {build.executionDuration
                ? displayDuration(build.executionDuration)
                : 'N/A'}
              {build.isExecuting ? '(and counting)' : ''}
              {build.exceededExecutionTimeout ? (
                <span className="warning">(exceeded timeout)</span>
              ) : (
                ''
              )}
              <span
                title={`Maximum execution duration: ${
                  build.executionTimeout
                    ? displayDuration(build.executionTimeout)
                    : 'N/A'
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
});
