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

import { useEffect } from 'react';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { constructArtifactName } from '@/common/services/resultdb';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';

// Redirects user to the new URL for raw artifact "/raw-artifact/:artifactName".
// Keep this around so we don't break the old URLs.
export function RawArtifactPage() {
  // We cannot use splats (i.e. "/raw/*") because react-router would return
  // decoded path and we can't recover the original path from that.
  // This is important because testId may contain encoded characters (e.g. %2f).
  const { invId, testId, resultId, artifactId } = useParams();

  if (!invId || !testId || !artifactId) {
    throw new Error('invId, testId, and artifactId must be set');
  }

  const artifactName = constructArtifactName({
    invocationId: invId,
    testId,
    resultId,
    artifactId,
  });
  useEffect(() => {
    // Use window.location.replace instead of useNavigate so the route isn't
    // intercepted by react-router.
    window.location.replace(getRawArtifactURLPath(artifactName));
  }, []);

  return <>Redirecting to the new raw artifact URL...</>;
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="raw">
      <RawArtifactPage />
    </RecoverableErrorBoundary>
  );
}
