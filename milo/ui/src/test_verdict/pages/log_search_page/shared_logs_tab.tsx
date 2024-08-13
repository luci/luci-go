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

import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { getAbsoluteStartEndTime } from '@/common/components/time_range_selector';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { useCurrentTime } from './contexts';
import { FormData } from './form_data';
import { InvocationLogDialog } from './log_list_dialog';
import { InvocationLogsTable } from './log_table';

// TODO (beining@):
// * link to log viewer.
// * only perform the search after some validation of the form.
export function SharedLogsTab() {
  const { project } = useParams();
  if (!project) {
    throw new Error('project must be set');
  }

  const [searchParams] = useSyncedSearchParams();

  const form = FormData.fromSearchParam(searchParams);

  const now = useCurrentTime();
  const { startTime, endTime } = getAbsoluteStartEndTime(searchParams, now);
  if (form && form.testIDStr.length > 0) {
    return <>no matching log</>;
  }
  return (
    form &&
    startTime &&
    endTime && (
      <>
        <InvocationLogsTable
          project={project}
          form={form}
          startTime={startTime}
          endTime={endTime}
        />
        <InvocationLogDialog
          project={project}
          form={form}
          startTime={startTime}
          endTime={endTime}
        />
      </>
    )
  );
}

export function Component() {
  useTabId('shared-logs');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="shared-logs">
      <SharedLogsTab />
    </RecoverableErrorBoundary>
  );
}
