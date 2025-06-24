// Copyright 2020 The LUCI Authors.
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

import { LinearProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { observer } from 'mobx-react-lite';
import { useEffect, useMemo } from 'react';
import { Helmet } from 'react-helmet';
import { useLocation, useNavigate, useParams } from 'react-router';

import {
  BUILD_FIELD_MASK,
  BUILD_STATUS_DISPLAY_MAP,
  PERM_BUILDS_GET,
} from '@/build/constants';
import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { OutputBuild } from '@/build/types';
import grayFavicon from '@/common/assets/favicons/gray-32.png';
import greenFavicon from '@/common/assets/favicons/green-32.png';
import miloFavicon from '@/common/assets/favicons/milo-32.png';
import purpleFavicon from '@/common/assets/favicons/purple-32.png';
import redFavicon from '@/common/assets/favicons/red-32.png';
import tealFavicon from '@/common/assets/favicons/teal-32.png';
import yellowFavicon from '@/common/assets/favicons/yellow-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { usePageSpecificConfig } from '@/common/components/page_config_state_provider';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { BUILD_STATUS_COLOR_THEME_MAP } from '@/common/constants/build';
import { UiPage } from '@/common/constants/view';
import { useFeatureFlag } from '@/common/feature_flags';
import { usePermCheck } from '@/common/hooks/perm_check';
import { Build as JsonBuild } from '@/common/services/buildbucket';
import { useStore } from '@/common/store';
import { InvocationProvider } from '@/common/store/invocation_state';
import { ContentGroup } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { NEW_TEST_INVESTIGATION_PAGE_FLAG } from '@/test_investigation/pages/features';
import {
  PERM_TEST_RESULTS_LIST_LIMITED,
  PERM_TEST_EXONERATIONS_LIST_LIMITED,
} from '@/test_verdict/constants/perms';

import { CountIndicator } from '../../../test_verdict/legacy/test_results_tab/count_indicator';

import { BuildIdBar } from './build_id_bar';
import { BuildLitEnvProvider } from './build_lit_env_provider';
import { ChangeConfigDialog } from './change_config_dialog';
import { BuildContextProvider } from './context';

const STATUS_FAVICON_MAP = Object.freeze({
  [Status.SCHEDULED]: grayFavicon,
  [Status.STARTED]: yellowFavicon,
  [Status.SUCCESS]: greenFavicon,
  [Status.FAILURE]: redFavicon,
  [Status.INFRA_FAILURE]: purpleFavicon,
  [Status.CANCELED]: tealFavicon,
});

export const BuildPage = observer(() => {
  const { project, bucket, builder, buildNumOrId } = useParams();

  if (!project || !bucket || !builder || !buildNumOrId) {
    throw new Error(
      'invariant violated: project, bucket, builder, buildNumOrId must be set',
    );
  }
  useEstablishProjectCtx(project);
  const builderId = { project, bucket, builder };

  const [showConfigDialog, setShowConfigDialog] = usePageSpecificConfig();
  const client = useBuildsClient();
  const req = buildNumOrId.startsWith('b')
    ? GetBuildRequest.fromPartial({
        id: buildNumOrId.slice('b'.length),
        mask: {
          fields: BUILD_FIELD_MASK,
        },
      })
    : GetBuildRequest.fromPartial({
        buildNumber: Number(buildNumOrId),
        builder: builderId,
        mask: {
          fields: BUILD_FIELD_MASK,
        },
      });
  const {
    data: build,
    isError,
    error,
    isPending,
  } = useQuery({
    ...client.GetBuild.query(req),
    select: (data) => data as OutputBuild,
    // Allow cache populated by <BuildPageShortLink /> to be used.
    staleTime: 5000,
  });
  if (isError) {
    // TODO(b/335065098): display a warning that the build might've expired if
    // the build is not found.
    throw error;
  }

  // TODO: remove the this section once the remaining usages of MobX is removed
  // from the build page.
  const store = useStore();
  useEffect(() => {
    store.buildPage.setParams({ project, bucket, builder }, buildNumOrId);
  }, [store, project, bucket, builder, buildNumOrId]);
  useEffect(() => {
    // When a defined build become an undefined build (e.g. due to a new build
    // is being displayed), we want to update the MobX store to reset the build
    // as well. Otherwise, components that rely on the mobx store will keep
    // displaying info for the old build.
    store.buildPage.setBuild(build ? (Build.toJSON(build) as JsonBuild) : null);
  }, [store, build]);

  const statusDisplay = build?.status
    ? BUILD_STATUS_DISPLAY_MAP[build.status]
    : 'loading';
  const documentTitle = `${statusDisplay} - ${builder} ${buildNumOrId}`;

  const faviconUrl = build?.status
    ? STATUS_FAVICON_MAP[build.status]
    : miloFavicon;

  const realm = `${project}:${bucket}`;
  const [canReadFullBuild] = usePermCheck(realm, PERM_BUILDS_GET);
  const [canReadResults] = usePermCheck(realm, PERM_TEST_RESULTS_LIST_LIMITED);
  const [canReadExonerations] = usePermCheck(
    realm,
    PERM_TEST_EXONERATIONS_LIST_LIMITED,
  );

  // Logic for redirection to new test-results UI page.
  const navigate = useNavigate();
  const location = useLocation();
  const isAutoRedirectEnabled = useFeatureFlag(
    NEW_TEST_INVESTIGATION_PAGE_FLAG,
  );
  const isTestResultsTabActive = location.pathname.endsWith('/test-results');
  const [searchParams] = useSyncedSearchParams();
  const forceLegacyView = searchParams.get('view') === 'legacy';

  const { testId, variantHash, variantDef } = useMemo(
    () => parseDeepLinkQuery(searchParams.get('q')),
    [searchParams],
  );

  useEffect(() => {
    if (
      forceLegacyView ||
      !isAutoRedirectEnabled ||
      !isTestResultsTabActive ||
      !build?.id
    ) {
      return;
    }

    // Deep link with a pre-computed variant hash.
    if (testId && variantHash) {
      const newPath =
        `/ui/test-investigate/invocations/build-${build.id}` +
        `/tests/${encodeURIComponent(testId)}` +
        `/variants/${variantHash}`;
      navigate(newPath, { replace: true });
      return;
    }

    // Deep link with variant key-value pairs.  This always goes to the invocation pae first,
    // and if the invocation page finds a single matching verdict it will redirect again to
    // the verdict page.
    if (testId || variantDef) {
      const search = new URLSearchParams();
      if (testId) {
        search.set('testId', testId);
      }
      if (variantDef) {
        Object.entries(variantDef).forEach(([key, value]) => {
          search.append('v', `${key}:${value}`);
        });
      }
      const newPath = `/ui/test-investigate/invocations/build-${build.id}?${search.toString()}`;
      navigate(newPath, { replace: true });
      return;
    }

    const newPath = `/ui/test-investigate/invocations/build-${build.id}`;
    navigate(newPath, { replace: true });
  }, [
    forceLegacyView,
    isAutoRedirectEnabled,
    isTestResultsTabActive,
    build?.id,
    testId,
    variantHash,
    variantDef,
    navigate,
  ]);

  if (!forceLegacyView && isAutoRedirectEnabled && isTestResultsTabActive) {
    return null;
  }

  return (
    <BuildContextProvider build={build}>
      <InvocationProvider value={store.buildPage.invocation}>
        <BuildLitEnvProvider>
          <Helmet>
            <title>{documentTitle}</title>
            <link rel="icon" href={faviconUrl} />
          </Helmet>
          <ChangeConfigDialog
            open={showConfigDialog}
            onClose={() => setShowConfigDialog(false)}
          />
          <BuildIdBar builderId={builderId} buildNumOrId={buildNumOrId} />
          <LinearProgress
            value={100}
            variant={isPending ? 'indeterminate' : 'determinate'}
            color={
              build ? BUILD_STATUS_COLOR_THEME_MAP[build.status] : 'primary'
            }
          />
          <AppRoutedTabs>
            <AppRoutedTab label="Overview" value="overview" to="overview" />
            <AppRoutedTab
              label="Test Results"
              value="test-results"
              to="test-results"
              hideWhenInactive={
                !store.buildPage.hasInvocation ||
                !canReadResults ||
                !canReadExonerations
              }
              icon={<CountIndicator />}
              iconPosition="end"
            />
            <AppRoutedTab
              label="Infra"
              value="infra"
              to="infra"
              hideWhenInactive={!canReadFullBuild}
            />
            <AppRoutedTab
              label="Related Builds"
              value="related-builds"
              to="related-builds"
              hideWhenInactive={!canReadFullBuild}
            />
            <AppRoutedTab
              label="Timeline"
              value="timeline"
              to="timeline"
              hideWhenInactive={!canReadFullBuild}
            />
            <AppRoutedTab
              label="Blamelist"
              value="blamelist"
              to="blamelist"
              hideWhenInactive={!canReadFullBuild}
            />
          </AppRoutedTabs>
        </BuildLitEnvProvider>
      </InvocationProvider>
    </BuildContextProvider>
  );
});

function parseDeepLinkQuery(q: string | null): {
  testId: string | null;
  variantHash: string | null;
  variantDef: Record<string, string> | null;
} {
  if (!q) {
    return { testId: null, variantHash: null, variantDef: null };
  }
  let testId: string | null = null;
  let variantHash: string | null = null;
  const variantDef: Record<string, string> = {};

  const parts = q.split(' ');
  for (const part of parts) {
    if (part.startsWith('ID:')) {
      testId = decodeURIComponent(part.slice('ID:'.length));
    } else if (part.startsWith('ExactID:')) {
      testId = decodeURIComponent(part.slice('ExactID:'.length));
    } else if (part.startsWith('VHash:')) {
      variantHash = decodeURIComponent(part.slice('VHash:'.length));
    } else if (part.startsWith('V:')) {
      const kvPair = part.slice('V:'.length);
      const firstEqIndex = kvPair.indexOf('=');
      if (firstEqIndex > 0) {
        const key = decodeURIComponent(kvPair.substring(0, firstEqIndex));
        const value = decodeURIComponent(kvPair.substring(firstEqIndex + 1));
        variantDef[key] = value;
      }
    }
  }

  const hasVariantDef = Object.keys(variantDef).length > 0;

  return {
    testId,
    variantHash: variantHash,
    variantDef: hasVariantDef ? variantDef : null,
  };
}

export function Component() {
  useDeclarePageId(UiPage.Builders);

  return (
    <ContentGroup group="build">
      <RecoverableErrorBoundary
        // See the documentation in the `<LoginPage />` for why we handle error
        // this way.
        key="build-long-link"
      >
        <BuildPage />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
