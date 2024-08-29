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

import { GrpcError } from '@chopsui/prpc-client';
import { Button, ButtonTypeMap, Link } from '@mui/material';
import { DefaultComponentProps } from '@mui/material/OverridableComponent';
import { useQuery } from '@tanstack/react-query';

import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants/rpc';
import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { renderBugUrlTemplate } from '@/common/tools/build_utils';
import { logging } from '@/common/tools/logging';
import { DeepNonNullable } from '@/generic_libs/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { GetProjectCfgRequest } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

export interface BuildBugButtonProps
  extends DefaultComponentProps<ButtonTypeMap> {
  readonly project: string;
  /**
   * The bug button will not be rendered until the build is populated.
   */
  // Making this optional allows the `GetProjectCfg` request to be sent without
  // waiting for the build query to resolve.
  readonly build?: DeepNonNullable<Pick<Build, 'id' | 'builder'>> | null;
}

export function BuildBugButton({
  project,
  build,
  ...props
}: BuildBugButtonProps) {
  const client = useMiloInternalClient();
  const { data, error } = useQuery({
    ...client.GetProjectCfg.query(
      GetProjectCfgRequest.fromPartial({
        project,
      }),
    ),
    select: (res) => {
      if (!res.bugUrlTemplate || !build) {
        return null;
      }
      return renderBugUrlTemplate(res.bugUrlTemplate, build);
    },
  });

  if (
    error instanceof GrpcError &&
    // Some users (e.g. CrOS partners) may have access to the build but not the
    // project configuration. Don't log permission errors to reduce noise.
    !POTENTIAL_PERM_ERROR_CODES.includes(error.code)
  ) {
    // Failing to get the bug link is Ok. Simply log the error here.
    logging.error('failed to get the custom bug link', error);
  }

  if (!data) {
    return <></>;
  }

  return (
    <Button {...props}>
      <Link
        target="_blank"
        rel="noopenner"
        href={data}
        sx={{ color: 'inherit' }}
      >
        Report bug in build
      </Link>
    </Button>
  );
}
