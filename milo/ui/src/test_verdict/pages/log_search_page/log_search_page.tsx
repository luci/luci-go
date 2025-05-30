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

import { Helmet } from 'react-helmet';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { ContentGroup } from '@/generic_libs/components/google_analytics';

import { LogSearch } from './log_search';

export function LogSearchPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project must be set');
  }
  useEstablishProjectCtx(project);

  return <LogSearch />;
}

export function Component() {
  useDeclarePageId(UiPage.LogSearch);

  return (
    <ContentGroup group="log-search">
      <Helmet>
        <title>Log search</title>
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="log-search"
      >
        <LogSearchPage />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
