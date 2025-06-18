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

import Link from '@mui/material/Link';
import { useQuery } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { logging } from '@/common/tools/logging';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

// Allowlist of hosts, used to validate URLs specified in the contents of link
// artifacts. If the URL specified in a link artifact is not in this allowlist,
// the original fetch URL for the artifact will be returned instead.
const LINK_ARTIFACT_HOST_ALLOWLIST = [
  'cros-test-analytics.appspot.com', // Testhaus logs
  'stainless.corp.google.com', // Stainless logs
  'tests.chromeos.goog', // Preferred. A hostname alias for Testhaus logs.
];

interface Props {
  artifact: Artifact;
  label?: string;
}

/**
 * ArtifactContentLink takes as a property an artifact that is a file with a URL in it,
 * it then downloads the contents of that file and generate a link to the URL in the file.
 */
export function ArtifactContentLink({ artifact, label }: Props) {
  const { identity } = useAuthState();

  const { data, error, isError } = useQuery({
    queryFn: async () => {
      const res = await fetch(
        urlSetSearchQueryParam(
          getRawArtifactURLPath(artifact.name),
          'n',
          ARTIFACT_LENGTH_LIMIT,
        ),
      );
      return res.text();
    },
    queryKey: [identity, 'fetch-raw-artifact', artifact.name],
  });

  if (isError) {
    throw error;
  }

  let url = getRawArtifactURLPath(artifact.name);
  if (data) {
    try {
      const dataUrl = new URL(data);
      const allowedProtocol = ['http:', 'https:'].includes(dataUrl.protocol);
      const allowedHost = LINK_ARTIFACT_HOST_ALLOWLIST.includes(dataUrl.host);
      if (allowedProtocol && allowedHost) {
        url = data;
      } else {
        logging.warn(
          `Invalid target URL for link artifact ${artifact.name} - ` +
            'returning the original fetch URL for the artifact instead',
        );
      }
    } catch (e) {
      logging.error('Failed to create url: ' + data);
      logging.error(e);
    }
  }
  return (
    <>
      {data && (
        <Link rel="noopenner" target="_blank" href={url}>
          {label ? label : artifact.artifactId}
        </Link>
      )}
    </>
  );
}
