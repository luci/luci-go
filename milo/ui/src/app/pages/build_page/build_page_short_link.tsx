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

import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useStore } from '@/common/store';
import { getBuildURLPath } from '@/common/tools/url_utils';

export const BuildPageShortLink = observer(() => {
  const { buildId, ['__luci_ui__-raw-*']: pathSuffix } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const store = useStore();

  if (!buildId) {
    throw new Error('invariant violated: buildId should be set');
  }

  useEffect(() => {
    store.buildPage.setParams(undefined, `b${buildId}`);
  }, [store, buildId]);

  const buildLoaded = Boolean(store.buildPage.build?.data);

  const urlSuffix =
    (pathSuffix ? `/${pathSuffix}` : '') + location.search + location.hash;
  useEffect(() => {
    // Redirect to the long link after the build is fetched.
    // The second check is noop but useful for type narrowing.
    if (!buildLoaded || !store.buildPage.build) {
      return;
    }
    const build = store.buildPage.build;
    if (build.data.number !== undefined) {
      store.buildPage.setBuildId(
        build.data.builder,
        build.data.number,
        build.data.id,
      );
    }
    const buildUrl = getBuildURLPath(build.data.builder, build.buildNumOrId);
    const newUrl = buildUrl + urlSuffix;

    // TODO(weiweilin): sinon is not able to mock useNavigate.
    // Add a unit test once we setup jest.
    navigate(newUrl, { replace: true });
  }, [buildLoaded, navigate, urlSuffix, store]);

  // Page will be redirected once the build is loaded.
  // Don't need to render anything.
  return <></>;
});

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="build-short-link">
    <BuildPageShortLink />
  </RecoverableErrorBoundary>
);
