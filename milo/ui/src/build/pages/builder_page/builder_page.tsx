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
import styled from '@emotion/styled';
import { Alert, AlertTitle, Grid, LinearProgress } from '@mui/material';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants';
import { usePrpcQuery } from '@/common/hooks/prpc_query';
import { parseLegacyBucketId } from '@/common/tools/build_utils';
import {
  BuilderMask_BuilderMaskType,
  BuildersClientImpl,
  GetBuilderRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_service.pb';

import { BuilderIdBar } from './builder_id_bar';
import { EndedBuildsSection } from './ended_builds_section';
import { MachinePoolSection } from './machine_pool_section';
import { PendingBuildsSection } from './pending_builds_section';
import { StartedBuildsSection } from './started_builds_section';
import { BuilderDescriptionSection } from './summary_section';
import { ViewsSection } from './views_section';

const ErrorDisplay = styled.pre({
  whiteSpace: 'pre-wrap',
  overflowWrap: 'break-word',
});

export function BuilderPage() {
  const { project, bucket, builder } = useParams();
  if (!project || !bucket || !builder) {
    throw new Error(
      'invariant violated: project, bucket, builder should be set',
    );
  }

  const builderId = {
    project,
    // If the bucket ID is a legacy ID, convert it to the new format.
    //
    // TODO(weiweilin): add a unit test once the pRPC query calls are
    // simplified.
    bucket: parseLegacyBucketId(bucket)?.bucket ?? bucket,
    builder,
  };

  const { data, error, isLoading } = usePrpcQuery({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildersClientImpl,
    method: 'GetBuilder',
    request: GetBuilderRequest.fromPartial({
      id: builderId,
      mask: {
        type: BuilderMask_BuilderMaskType.ALL,
      },
    }),
    options: {
      select: (res) => ({
        swarmingHost: res.config!.swarmingHost,
        // Convert dimensions to StringPair[] and remove expirations.
        dimensions:
          res.config!.dimensions?.map((dim) => {
            const parts = dim.split(':', 3);
            if (parts.length === 3) {
              return { key: parts[1], value: parts[2] };
            }
            return { key: parts[0], value: parts[1] };
          }) || [],
        descriptionHtml: res.config!.descriptionHtml,
        metadata: res.metadata,
        // TODO guterman: check reported date and whether it's expired
      }),
    },
  });

  if (error && !(error instanceof GrpcError)) {
    throw error;
  }

  return (
    <>
      <PageMeta
        project={project}
        selectedPage={UiPage.Builders}
        title={`${builderId.builder} | Builder`}
      />
      <BuilderIdBar
        builderId={builderId}
        healthStatus={data?.metadata?.health}
      />
      <LinearProgress
        value={100}
        variant={isLoading ? 'indeterminate' : 'determinate'}
        color="primary"
      />
      <Grid container spacing={2} sx={{ padding: '0 16px' }}>
        {error instanceof GrpcError && (
          <Grid item md={12}>
            <Alert severity="warning" sx={{ mt: 2 }}>
              <AlertTitle>
                Failed to query the builder. If you can see recent builds, the
                builder might have been deleted recently.
              </AlertTitle>
              <ErrorDisplay>{`Original Error:\n${error.message}`}</ErrorDisplay>
            </Alert>
          </Grid>
        )}
        {data?.descriptionHtml && (
          <Grid item md={12}>
            <BuilderDescriptionSection descriptionHtml={data.descriptionHtml} />
          </Grid>
        )}
        {data?.swarmingHost && (
          <Grid item md={5}>
            <MachinePoolSection
              swarmingHost={data.swarmingHost}
              dimensions={data.dimensions}
            />
          </Grid>
        )}
        <Grid item md={2}>
          <StartedBuildsSection builderId={builderId} />
        </Grid>
        <Grid item md={2}>
          <PendingBuildsSection builderId={builderId} />
        </Grid>
        <Grid item md={3}>
          <ViewsSection builderId={builderId} />
        </Grid>
        <Grid item md={12}>
          <EndedBuildsSection builderId={builderId} />
        </Grid>
      </Grid>
    </>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="builder">
    <BuilderPage />
  </RecoverableErrorBoundary>
);
