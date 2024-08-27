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

import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { Alert, AlertTitle, Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { customElement } from 'lit/decorators.js';
import { useMemo } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/test';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { ReactLitElement } from '@/generic_libs/components/react_lit_element';
import { consumer } from '@/generic_libs/tools/lit_context';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

import { ArtifactContextProvider, useResultName } from '../context';
import { consumeResultName } from '../lit_context';

const EXTRA_BYTES = 8;

export interface TextArtifactProps {
  readonly artifactId: string;
  readonly invLevel?: boolean;
}

export function TextArtifact({
  artifactId,
  invLevel = false,
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

  const { data, isError, error, isLoading } = useQuery({
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
      if (hasMore) {
        // When the content is longer than `ARTIFACT_LENGTH_LIMIT`, we will
        // append `'...'` and a raw link at the end.
        // Remove the last character so we don't ends up showing `'...'` with
        // the full content when the content has exactly
        // `ARTIFACT_LENGTH_LIMIT + EXTRA_BYTES` bytes.
        return [content.slice(0, content.length - 1), hasMore] as const;
      }
      return [content, false] as const;
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
  if (isLoading) {
    return (
      <>
        Loading <DotSpinner />
      </>
    );
  }
  const [content, hasMore] = data;
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
      {content}
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

/**
 * Renders a text artifact.
 */
@customElement('text-artifact')
@consumer
export class TextArtifactElement extends ReactLitElement {
  static get properties() {
    return {
      artifactId: {
        attribute: 'artifact-id',
        type: String,
      },
      invLevel: {
        attribute: 'inv-level',
        type: Boolean,
      },
      resultName: {
        state: true,
      },
    };
  }

  private _artifactId = '';
  get artifactId() {
    return this._artifactId;
  }
  set artifactId(newVal: string) {
    if (newVal === this._artifactId) {
      return;
    }
    const oldVal = this._artifactId;
    this._artifactId = newVal;
    this.requestUpdate('artifactId', oldVal);
  }

  private _invLevel = false;
  get invLevel() {
    return this._invLevel;
  }
  set invLevel(newVal: boolean) {
    if (newVal === this._invLevel) {
      return;
    }
    const oldVal = this._invLevel;
    this._invLevel = newVal;
    this.requestUpdate('invLevel', oldVal);
  }

  private _resultName: string | null = null;
  get resultName() {
    return this._resultName;
  }
  /**
   * Allows the result name to be provided by a Lit context provider.
   *
   * This makes it easier to use the artifact tags in a Lit component during the
   * transition phase.
   *
   * When the result name is provided this way, the React component is rendered
   * to the directly under the closest `<PortalScope />` ancestor in the React
   * virtual DOM tree. Make sure all the context required by the artifact tags
   * are available to that `<PortalScope />`.
   */
  @consumeResultName()
  set resultName(newVal: string | null) {
    if (newVal === this._resultName) {
      return;
    }
    const oldVal = this._resultName;
    this._resultName = newVal;
    this.requestUpdate('resultName', oldVal);
  }

  private cache: EmotionCache | undefined = undefined;

  renderReact() {
    if (this.resultName !== null) {
      // When the result name is provided by a lit context provider, its very
      // likely that this element is in a shadow DOM. Provide emotion cache at
      // this level so the CSS styles are carried over.
      if (!this.cache) {
        this.cache = createCache({
          key: 'text-artifact',
          container: this,
        });
      }
      return (
        <CacheProvider value={this.cache}>
          {/* If the result name is provided by a lit context provider, bridge
           ** it to a React context so it can be consumed. */}
          <ArtifactContextProvider resultName={this.resultName}>
            <TextArtifact
              artifactId={this.artifactId}
              invLevel={this.invLevel}
            />
          </ArtifactContextProvider>
        </CacheProvider>
      );
    }

    return (
      <TextArtifact artifactId={this.artifactId} invLevel={this.invLevel} />
    );
  }
}
