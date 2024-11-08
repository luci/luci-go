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

// import { nanoid } from 'nanoid';
import { Fragment, ReactNode } from 'react';

import { FailureGroup } from '@/clusters/tools/failures_tools';

import FailuresTableRows from './failures_table_rows/failures_table_rows';

const renderGroup = (
  project: string,
  group: FailureGroup,
  selectedVariantGroups: string[],
): ReactNode => {
  return (
    <Fragment>
      <FailuresTableRows
        project={project}
        group={group}
        selectedVariantGroups={selectedVariantGroups}
      >
        {group.children.map((childGroup) => (
          <Fragment key={childGroup.id}>
            {renderGroup(project, childGroup, selectedVariantGroups)}
          </Fragment>
        ))}
      </FailuresTableRows>
    </Fragment>
  );
};

interface Props {
  project: string;
  group: FailureGroup;
  selectedVariantGroups: string[];
}

const FailuresTableGroup = ({
  project,
  group,
  selectedVariantGroups,
}: Props) => {
  return <>{renderGroup(project, group, selectedVariantGroups)}</>;
};

export default FailuresTableGroup;
