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

import { Fragment } from 'react';

import { OutputTestVariantBranch } from '@/analysis/types';

import { SegmentSpan } from './segment_span';
import { StartPointSpan } from './start_point_span';

export interface RowProps {
  readonly testVariantBranch: OutputTestVariantBranch;
}

export function Row({ testVariantBranch }: RowProps) {
  return (
    <>
      {testVariantBranch.segments.map((seg, i, segs) => (
        <Fragment key={seg.endPosition}>
          <StartPointSpan
            testVariantBranch={testVariantBranch}
            segment={seg}
            prevSegment={segs[i + 1] || null}
            // Alternate between top and bottom to avoid start point spans
            // overlapping each other.
            position={i % 2 === 0 ? 'top' : 'bottom'}
          />
          <SegmentSpan testVariantBranch={testVariantBranch} segment={seg} />
        </Fragment>
      ))}
    </>
  );
}
