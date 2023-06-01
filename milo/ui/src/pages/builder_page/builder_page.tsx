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

import { Grid, LinearProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';

import {
  useAuthState,
  useGetAccessToken,
} from '../../components/auth_state_provider';
import { PrpcClientExt } from '../../libs/prpc_client_ext';
import { BuildersService, GetBuilderRequest } from '../../services/buildbucket';
import { BuilderIdBar } from './builder_id_bar';
import { EndedBuildsSection } from './ended_builds_section';
import { MachinePoolSection } from './machine_pool_section';
import { PendingBuildsSection } from './pending_builds_section';
import { StartedBuildsSection } from './started_builds_section';

function useBuilder(req: GetBuilderRequest) {
  const { identity } = useAuthState();
  const getAccessToken = useGetAccessToken();
  return useQuery({
    queryKey: [identity, BuildersService.SERVICE, 'GetBuilder', req],
    queryFn: async () => {
      const buildersService = new BuildersService(
        new PrpcClientExt({ host: CONFIGS.BUILDBUCKET.HOST }, getAccessToken)
      );
      const res = await buildersService.getBuilder(
        req,
        // Let react-query manage caching.
        { acceptCache: false, skipUpdate: true }
      );
      return {
        swarmingHost: res.config.swarmingHost,
        // Convert dimensions to StringPair[] and remove expirations.
        dimensions:
          res.config.dimensions?.map((dim) => {
            const parts = dim.split(':', 3);
            if (parts.length === 3) {
              return { key: parts[1], value: parts[2] };
            }
            return { key: parts[0], value: parts[1] };
          }) || [],
      };
    },
  });
}

export function BuilderPage() {
  const { project, bucket, builder } = useParams();
  if (!project || !bucket || !builder) {
    throw new Error(
      'invariant violated: project, bucket, builder should be set'
    );
  }

  const builderId = { project, bucket, builder };

  const { data, error, isError, isLoading } = useBuilder({ id: builderId });
  if (isError) {
    throw error;
  }

  return (
    <>
      <BuilderIdBar builderId={builderId} />
      <LinearProgress
        value={100}
        variant={isLoading ? 'indeterminate' : 'determinate'}
        color="primary"
      />
      {!isLoading && (
        <Grid container spacing={2} sx={{ padding: '0 16px' }}>
          {data.swarmingHost && (
            <Grid item md={4}>
              <MachinePoolSection
                swarmingHost={data.swarmingHost}
                dimensions={data.dimensions}
              />
            </Grid>
          )}
          <Grid item md={2}>
            <StartedBuildsSection builderId={builderId} />
          </Grid>
          <Grid item md={3}>
            <PendingBuildsSection builderId={builderId} />
          </Grid>
          <Grid item md={12}>
            <EndedBuildsSection builderId={builderId} />
          </Grid>
        </Grid>
      )}
    </>
  );
}
