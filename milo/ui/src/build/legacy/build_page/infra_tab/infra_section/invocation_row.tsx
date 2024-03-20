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

import { MiloLink } from '@/common/components/link';
import { BuildInfraResultdb } from '@/common/services/buildbucket';
import { getInvocationLink } from '@/common/tools/build_utils';

export interface InvocationRowProps {
  readonly resultdb?: BuildInfraResultdb;
}

export function InvocationRow({ resultdb }: InvocationRowProps) {
  const invocationLink = resultdb?.invocation
    ? getInvocationLink(resultdb.invocation)
    : null;

  return (
    <tr>
      <td>ResultDB Invocation:</td>
      <td>
        {invocationLink ? (
          <MiloLink link={invocationLink} target="_blank" />
        ) : (
          'None'
        )}
      </td>
    </tr>
  );
}
