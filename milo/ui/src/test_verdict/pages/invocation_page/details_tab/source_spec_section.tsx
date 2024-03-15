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

import { ChangelistLink } from '@/gitiles/components/changelist_link';
import { SourceSpec } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

import { GitilesCommitRow } from './gitiles_commit_row';

export interface SourceSpecSectionProps {
  readonly sourceSpec: SourceSpec | undefined;
}

export function SourceSpecSection({ sourceSpec }: SourceSpecSectionProps) {
  return (
    <>
      {sourceSpec === undefined && (
        <>
          <p>Sources have not been specified for this invocation.</p>
        </>
      )}
      {sourceSpec?.sources && (
        <>
          <table>
            <tbody>
              {sourceSpec.sources.gitilesCommit && (
                <GitilesCommitRow commit={sourceSpec.sources.gitilesCommit} />
              )}
              {sourceSpec.sources.changelists.length > 0 && (
                <tr>
                  <td>Changelists:</td>
                  <td>
                    {sourceSpec.sources.changelists.map((cl, i) => (
                      <Fragment key={i}>
                        {i > 0 ? `, ` : ``}
                        <ChangelistLink changelist={cl} />
                      </Fragment>
                    ))}
                  </td>
                </tr>
              )}
              {sourceSpec.sources.isDirty && (
                <tr>
                  <td>Is Dirty:</td>
                  <td>Yes</td>
                </tr>
              )}
            </tbody>
          </table>
        </>
      )}
      {sourceSpec?.inherit && (
        <p>
          This invocation inherits its sources from its including (parent)
          invocation.
        </p>
      )}
    </>
  );
}
