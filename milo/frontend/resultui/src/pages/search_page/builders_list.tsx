// Copyright 2022 The LUCI Authors.
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

import { getURLPathForBuilder } from '../../libs/build_utils';
import { BuilderID } from '../../services/buildbucket';

export interface BuildersListProps {
  readonly groupedBuilders: { [bucketId: string]: BuilderID[] | undefined };
}

export const BuildersList = observer(({ groupedBuilders }: BuildersListProps) => {
  return (
    <>
      {Object.entries(groupedBuilders).map(([bucketId, builders]) => (
        <div key={bucketId}>
          <h3>{bucketId}</h3>
          <ul>
            {builders?.map((builder) => (
              <li key={builder.builder}>
                <a href={getURLPathForBuilder(builder)}>{builder.builder}</a>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </>
  );
});
