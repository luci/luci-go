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

import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { useBuild } from '../hooks';

import { RevisionRow } from './revision_row';

export function OutputSection() {
  const build = useBuild();
  const output = build?.output;
  if (!output?.gitilesCommit) {
    return <></>;
  }

  return (
    <>
      <h3>Output</h3>
      <table>
        <tbody>
          <RevisionRow commit={GitilesCommit.fromJSON(output.gitilesCommit)} />
        </tbody>
      </table>
    </>
  );
}
