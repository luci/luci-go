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
import { useQuery } from '@tanstack/react-query';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router-dom';

import { useBuildersClient } from '@/build/hooks/prpc_clients';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { usePageId, useProject } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { parseLegacyBucketId } from '@/common/tools/build_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  BuilderMask_BuilderMaskType,
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
    throw new Error('invariant violated: project, bucket, builder must be set');
  }
  useProject(project);

  const builderId = {
    project,
    // If the bucket ID is a legacy ID, convert it to the new format.
    //
    // TODO(weiweilin): add a unit test once the pRPC query calls are
    // simplified.
    bucket: parseLegacyBucketId(bucket)?.bucket ?? bucket,
    builder,
  };

  const client = useBuildersClient();
  const { data, error, isLoading } = useQuery({
    ...client.GetBuilder.query(
      GetBuilderRequest.fromPartial({
        id: builderId,
        mask: {
          type: BuilderMask_BuilderMaskType.ALL,
        },
      }),
    ),
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
  });

  if (error && !(error instanceof GrpcError)) {
    throw error;
  }

  return (
    <>
      <Helmet>
        <title>{builderId.builder} | Builders</title>
      </Helmet>
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

export function Component() {
  usePageId(UiPage.Builders);

  return (
    <TrackLeafRoutePageView contentGroup="builder">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="builder"
      >
        <BuilderPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
