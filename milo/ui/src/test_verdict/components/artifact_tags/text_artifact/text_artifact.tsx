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

import { Alert, AlertTitle, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { ANSIText } from '@/common/components/ansi_text';
import { useAuthState } from '@/common/components/auth_state_provider';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

import { useResultName } from '../context';

const EXTRA_BYTES = 8;

export interface TextArtifactProps {
  readonly artifactId: string;
  readonly invLevel?: boolean;
  readonly experimentalANSISupport?: boolean;
}

export function TextArtifact({
  artifactId,
  invLevel = false,
  experimentalANSISupport = false,
}: TextArtifactProps) {
  const { identity } = useAuthState();
  const resultName = useResultName();

  const fetchUrl = useMemo(() => {
    let artifactName = '';
    if (invLevel) {
      const invName = resultName.slice(0, resultName.indexOf('/tests/'));
      artifactName = `${invName}/artifacts/${artifactId}`;
    } else {
      artifactName = `${resultName}/artifacts/${artifactId}`;
    }
    return getRawArtifactURLPath(artifactName);
  }, [invLevel, artifactId, resultName]);

  const { data, isError, error, isPending } = useQuery({
    queryKey: [identity, 'fetch-text-artifact', fetchUrl],
    queryFn: async () => {
      const url = urlSetSearchQueryParam(
        fetchUrl,
        'n',
        // Fetch a few more bytes so we know whether the content exceeds the
        // length limit or has exactly `ARTIFACT_LENGTH_LIMIT` bytes.
        ARTIFACT_LENGTH_LIMIT + EXTRA_BYTES,
      );
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(await response.text());
      }
      const content = await response.text();
      const contentLengthHeader = response.headers.get('Content-Length');
      const contentLength = contentLengthHeader
        ? parseInt(contentLengthHeader)
        : 0;
      const hasMore = contentLength > ARTIFACT_LENGTH_LIMIT;
      return {
        contentType: response.headers.get('Content-Type'),
        // When the content is longer than `ARTIFACT_LENGTH_LIMIT`, we will
        // append `'...'` and a raw link at the end.
        // Remove the last character so we don't ends up showing `'...'` with
        // the full content when the content has exactly
        // `ARTIFACT_LENGTH_LIMIT + EXTRA_BYTES` bytes.
        content: hasMore ? content.slice(0, content.length - 1) : content,
        hasMore,
      };
    },
  });
  if (isError) {
    return (
      <Alert severity="error">
        <AlertTitle>
          Failed to load {invLevel ? 'inv-level' : ''} artifact: {artifactId}
        </AlertTitle>
        <pre>{error instanceof Error ? error.message : `${error}`}</pre>
      </Alert>
    );
  }
  if (isPending) {
    return (
      <>
        Loading <DotSpinner />
      </>
    );
  }

  const { contentType, content, hasMore } = data;
  if (content === '') {
    const label = invLevel ? 'Inv-level artifact' : 'Artifact';
    return (
      <>
        {label}
        <Link href={fetchUrl} target="_blank" rel="noopenner">
          {artifactId}
        </Link>
        is empty.
      </>
    );
  }

  return (
    <pre data-testid="text-artifact-content">
      {experimentalANSISupport && contentType === 'text/x-ansi' ? (
        <ANSIText content={content} />
      ) : (
        content
      )}
      {hasMore ? (
        <>
          ... (truncated, view the full content{' '}
          <Link href={fetchUrl} target="_blank" rel="noopenner">
            here
          </Link>
          )
        </>
      ) : (
        <></>
      )}
    </pre>
  );
}
