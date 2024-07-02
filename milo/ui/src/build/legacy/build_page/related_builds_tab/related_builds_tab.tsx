// Copyright 2022 The LUCI Authors.
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

import { CircularProgress } from '@mui/material';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useTabId } from '@/generic_libs/components/routed_tabs';

import { useBuild } from '../context';

import { RelatedBuildsDisplay } from './related_builds_display';

export function RelatedBuildsTab() {
  const build = useBuild();

  if (!build) {
    return <CircularProgress sx={{ margin: '10px' }} />;
  }

  return <RelatedBuildsDisplay build={build} />;
}

export function Component() {
  useTabId('related-builds');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="related-builds">
      <RelatedBuildsTab />
    </RecoverableErrorBoundary>
  );
}
