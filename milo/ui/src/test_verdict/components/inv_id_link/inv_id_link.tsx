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

import { Link } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';

import { parseInvId } from '@/common/tools/invocation_utils';
import {
  getBuildURLPathFromBuildId,
  getSwarmingTaskURL,
} from '@/common/tools/url_utils';

export interface InvIdLinkProps {
  readonly invId: string;
}

export function InvIdLink({ invId: invId }: InvIdLinkProps) {
  const parsedInvId = parseInvId(invId);
  let link = `/ui/inv/${invId}`;
  switch (parsedInvId.type) {
    case 'build':
      link = getBuildURLPathFromBuildId(parsedInvId.buildId);
      break;
    case 'swarming-task':
      link = getSwarmingTaskURL(parsedInvId.swarmingHost, parsedInvId.taskId);
      break;
    default:
      break;
  }

  return (
    <Link component={RouterLink} to={link}>
      {invId}
    </Link>
  );
}
