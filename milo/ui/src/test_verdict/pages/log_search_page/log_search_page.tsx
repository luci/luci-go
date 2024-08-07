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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';

import { LogSearch } from './log_search';

export function LogSearchPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('project must be set');
  }

  return (
    <>
      <PageMeta
        selectedPage={UiPage.LogSearch}
        title="log search"
        project={project}
      ></PageMeta>
      <LogSearch />
    </>
  );
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="log-search">
      <LogSearchPage />
    </RecoverableErrorBoundary>
  );
}
