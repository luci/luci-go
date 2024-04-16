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

import { Box, CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';

import { BUILD_FIELD_MASK } from '@/build/constants';
import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { OutputBuild } from '@/build/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { getBuildURLPathFromBuildData } from '@/common/tools/url_utils';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

export function BuildPageShortLink() {
  const { buildId, ['__luci_ui__-raw-*']: pathSuffix } = useParams();
  const location = useLocation();
  const navigate = useNavigate();

  if (!buildId) {
    throw new Error('invariant violated: buildId should be set');
  }

  // TODO: cache the build so we don't need to fetch the build again after
  // redirection.
  const client = useBuildsClient();
  const {
    data: build,
    isError,
    error,
  } = useQuery({
    ...client.GetBuild.query(
      GetBuildRequest.fromPartial({
        id: buildId,
        mask: {
          fields: BUILD_FIELD_MASK,
        },
      }),
    ),
    select: (data) => data as OutputBuild,
  });
  if (isError) {
    // TODO(b/335065098): display a warning that the build might've expired if
    // the build is not found.
    throw error;
  }

  const urlSuffix =
    (pathSuffix ? `/${pathSuffix}` : '') + location.search + location.hash;

  // Redirect to the long link after the build is fetched.
  useEffect(() => {
    if (!build) {
      return;
    }
    const buildUrl = getBuildURLPathFromBuildData(build);
    const newUrl = buildUrl + urlSuffix;

    // TODO(weiweilin): sinon is not able to mock useNavigate.
    // Add a unit test once we setup jest.
    navigate(newUrl, { replace: true });
  }, [build, urlSuffix, navigate]);

  return (
    <Box display="flex" justifyContent="center" alignItems="center">
      <CircularProgress />
    </Box>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="build-short-link">
    <BuildPageShortLink />
  </RecoverableErrorBoundary>
);
