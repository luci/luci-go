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
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { RecentRegressions } from './recent_regressions';

export function RecentRegressionsPage() {
  const { project } = useParams();
  if (!project) {
    throw new Error('project must be set');
  }

  return (
    <>
      <PageMeta
        selectedPage={UiPage.RecentRegressions}
        title="recent regressions"
        project={project}
      ></PageMeta>
      <RecentRegressions project={project} />
    </>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="recent-regressions">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="recent-regressions"
      >
        <RecentRegressionsPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
