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

import { Helmet } from 'react-helmet';
import { Outlet, useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { ContentGroup } from '@/generic_libs/components/google_analytics';

export const BisectionLayout = () => {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project must be set');
  }
  useEstablishProjectCtx(project);

  return <Outlet />;
};

export function Component() {
  useDeclarePageId(UiPage.Bisection);

  return (
    <ContentGroup group="bisection">
      <Helmet>
        <title>Bisection</title>
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="bisection"
      >
        <BisectionLayout />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
