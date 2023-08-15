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
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';

import { useStore } from '@/common/store';
import { getBuildURLPath } from '@/common/tools/url_utils';

export const BuildPageShortLink = observer(() => {
  const { buildId, ['*']: pathSuffix } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const store = useStore();

  if (!buildId) {
    throw new Error('invariant violated: buildId should be set');
  }

  useEffect(() => {
    store.buildPage.setParams(undefined, `b${buildId}`);
  }, [store, buildId]);

  const buildLoaded = Boolean(store.buildPage.build?.data);

  useEffect(() => {
    // Redirect to the long link after the build is fetched.
    if (!buildLoaded) {
      return;
    }
    const build = store.buildPage.build!;
    if (build.data.number !== undefined) {
      store.buildPage.setBuildId(
        build.data.builder,
        build.data.number,
        build.data.id
      );
    }
    const buildUrl = getBuildURLPath(build.data.builder, build.buildNumOrId);
    const searchString = searchParams.toString();
    const newUrl = `${buildUrl}${pathSuffix ? `/${pathSuffix}` : ''}${
      searchString ? `?${searchParams}` : ''
    }`;

    // TODO(weiweilin): sinon is not able to mock useNavigate.
    // Add a unit test once we setup jest.
    navigate(newUrl, { replace: true });
  }, [buildLoaded]);

  // Page will be redirected once the build is loaded.
  // Don't need to render anything.
  return <></>;
});
