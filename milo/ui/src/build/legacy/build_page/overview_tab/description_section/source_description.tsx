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
import { CommitLink } from '@/gitiles/components/commit_link';
import { getGerritChangeURL } from '@/gitiles/tools/utils';
import {
  GerritChange,
  GitilesCommit,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface SourceDescriptionProps {
  readonly commit: GitilesCommit;
  readonly changes: readonly GerritChange[];
}

export function SourceDescription({ commit, changes }: SourceDescriptionProps) {
  return (
    <>
      from <CommitLink commit={commit} />
      {changes.length > 0 ? (
        <>
          {' '}
          with changes:{' '}
          {changes.map((c, i) => (
            <Fragment key={getGerritChangeURL(c)}>
              {i === 0 ? '' : ', '}
              <ChangelistLink changelist={c} />
            </Fragment>
          ))}
        </>
      ) : (
        <></>
      )}
    </>
  );
}
