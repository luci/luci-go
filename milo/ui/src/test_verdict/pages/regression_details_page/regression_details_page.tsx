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

import { useParams } from 'react-router-dom';

import { ParsedTestVariantBranchName } from '@/analysis/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta, usePageId, useProject } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ChangepointPredicate } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';

import { RegressionDetails } from './regression_details';

export function RegressionDetailsPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project must be set');
  }
  useProject(project);

  // TODO(b/321110247): once we have a stable regression group ID, pass it via
  // the path params instead of passing a bunch of filters via search query
  // params.
  const [searchParams] = useSyncedSearchParams();
  const tvb = searchParams.get('tvb');
  const nsp = searchParams.get('nsp');
  const sh = searchParams.get('sh');
  const cp = searchParams.get('cp') || '{}';
  if (!tvb || !nsp || !sh) {
    throw new Error(
      'tvb (test variant branch), nsp (nominal start position), sh (start hour) must be set in the search param',
    );
  }

  const testVariantBranch = ParsedTestVariantBranchName.fromString(tvb);
  const nominalStartPosition = nsp;
  const startHour = sh;
  const predicate = ChangepointPredicate.fromJSON(JSON.parse(cp));

  return (
    <>
      <PageMeta title="regression details"></PageMeta>
      <RegressionDetails
        testVariantBranch={testVariantBranch}
        nominalStartPosition={nominalStartPosition}
        startHour={startHour}
        predicate={predicate}
      />
    </>
  );
}

export function Component() {
  usePageId(UiPage.RegressionDetails);

  return (
    <TrackLeafRoutePageView
      contentGroup="regression-details"
      searchParamKeys={['tvb', 'nsp', 'sh', 'cp']}
    >
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="regression-details"
      >
        <RegressionDetailsPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
