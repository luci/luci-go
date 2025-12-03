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

import { Box, Link } from '@mui/material';

import { TestCheckDescriptionOption } from '@/proto/turboci/data/test/v1/test_check_description_option.pb';
import { TestCheckSummaryResult } from '@/proto/turboci/data/test/v1/test_check_summary_result.pb';

import { DetailRow } from './detail_row';

export function TestCheckDescriptionOptionDetails({
  data,
}: {
  data: TestCheckDescriptionOption;
}) {
  return (
    <Box>
      <DetailRow label="Title" value={data.title} />
      <DetailRow label="Description" value={data.displayMessage?.message} />
    </Box>
  );
}

export function TestCheckSummaryResultDetails({
  data,
}: {
  data: TestCheckSummaryResult;
}) {
  const passed = data.successCount ?? 0;
  const total = data.testCount ?? 0;
  const failed = data.failureCount ?? 0;
  const skipped = data.skipCount ?? 0;

  return (
    <Box>
      <DetailRow label="Success" value={data.success ? 'True' : 'False'} />
      <DetailRow label="Message" value={data.displayMessage?.message} />
      <DetailRow
        label="Tests"
        value={`${passed} / ${total} tests passed (${failed} failed, ${skipped} skipped)`}
      />
      {data.viewUrl && (
        <DetailRow
          label="View URL"
          value={
            <Link href={data.viewUrl} target="_blank" rel="noopener">
              {data.viewUrl}
            </Link>
          }
        />
      )}
    </Box>
  );
}
