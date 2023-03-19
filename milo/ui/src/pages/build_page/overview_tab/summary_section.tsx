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

import { observer } from 'mobx-react-lite';

import { RelativeTimestamp } from '../../../components/relative_timestamp';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP } from '../../../libs/constants';
import { renderMarkdown } from '../../../libs/markdown_utils';
import { BuildStatus } from '../../../services/buildbucket';
import { useStore } from '../../../store';

export const SummarySection = observer(() => {
  const store = useStore();
  const build = store.buildPage.build;
  if (!build) {
    return <></>;
  }

  const scheduledCancelTime = build.cancelTime
    // We have gracePeriod since Feb 2021. It's safe to use ! since all new
    // (not-terminated) builds should have gracePeriod defined.
    ?.plus(build.gracePeriod!)
    // Add min_update_interval (currently always 30s).
    // TODO(crbug/1299302): read min_update_interval from buildbucket.
    .plus({ seconds: 30 });

  return (
    <>
      {build?.isCanary && [BuildStatus.Failure, BuildStatus.InfraFailure].includes(build.data.status) && (
        <div css={{ padding: '5px', backgroundColor: 'var(--warning-color)', fontWeight: 500 }}>
          WARNING: This build ran on a canary version of LUCI. If you suspect it failed due to infra, retry the build.
          Next time it may use the non-canary version.
        </div>
      )}
      {scheduledCancelTime && (
        <div
          css={{
            backgroundColor: 'var(--canceled-bg-color)',
            fontWeight: 500,
            padding: '5px',
          }}
        >
          This build was scheduled to be canceled by {build.data.canceledBy || 'unknown'}
          <RelativeTimestamp timestamp={scheduledCancelTime} />
        </div>
      )}
      <div
        css={{
          padding: '0 10px',
          clear: 'both',
          overflowWrap: 'break-word',
          '& pre': {
            whiteSpace: 'pre-wrap',
            overflowWrap: 'break-word',
            fontSize: '12px',
          },
          '& *': {
            marginBlock: '10px',
          },
        }}
        className={`${BUILD_STATUS_CLASS_MAP[build.data.status]}-bg`}
      >
        {build?.data.summaryMarkdown ? (
          <div dangerouslySetInnerHTML={{ __html: renderMarkdown(build.data.summaryMarkdown) }} />
        ) : (
          <div css={{ fontWeight: 500 }}>Build {BUILD_STATUS_DISPLAY_MAP[build.data.status] || 'status unknown'}</div>
        )}
      </div>
    </>
  );
});
