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

import { Link } from '@mui/material';

import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ArtifactContentLink } from './artifact_content_link';

export interface ArtifactLinkProps {
  readonly artifact: Artifact;
  readonly label?: string;
}

export function ArtifactLink({ artifact, label }: ArtifactLinkProps) {
  if (artifact.contentType === 'text/x-uri') {
    return <ArtifactContentLink artifact={artifact} label={label} />;
  }

  return (
    <Link
      href={getRawArtifactURLPath(artifact.name)}
      target="_blank"
      rel="noopenner"
    >
      {label || artifact.artifactId}
    </Link>
  );
}
