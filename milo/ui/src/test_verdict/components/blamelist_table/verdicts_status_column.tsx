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

import { Skeleton, TableCell } from '@mui/material';
import { useMemo } from 'react';

import { OutputTestVerdict } from '@/analysis/types';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { VerdictSetStatus } from '@/test_verdict/components/verdict_set_status';

export function VerdictsStatusHeadCell() {
  return (
    <TableCell
      width="1px"
      title="Status of test verdicts associated with this commit."
    >
      Verd.
    </TableCell>
  );
}

export interface VerdictsStatusContentCellProps {
  readonly testVerdicts: readonly OutputTestVerdict[] | null;
}

export function VerdictStatusesContentCell({
  testVerdicts,
}: VerdictsStatusContentCellProps) {
  const counts = useMemo(() => {
    const c = {
      [TestVariantStatus.UNEXPECTED]: 0,
      [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 0,
      [TestVariantStatus.FLAKY]: 0,
      [TestVariantStatus.EXONERATED]: 0,
      [TestVariantStatus.EXPECTED]: 0,
    };
    for (const tv of testVerdicts || []) {
      c[tv.status] += 1;
    }
    return c;
  }, [testVerdicts]);

  return (
    <TableCell sx={{ minWidth: '30px' }}>
      {testVerdicts ? (
        <VerdictSetStatus counts={counts} />
      ) : (
        <Skeleton variant="circular" height={24} width={24} />
      )}
    </TableCell>
  );
}
