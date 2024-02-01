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

import { MobxLitElement } from '@adobe/lit-mobx';
import * as Diff2Html from 'diff2html';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '@/generic_libs/components/dot_spinner';
import '@/common/components/status_bar';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/test';
import {
  ArtifactIdentifier,
  constructArtifactName,
} from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { commonStyles } from '@/common/styles/stylesheets';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

import { consumeArtifactIdent } from './artifact_page_layout';

/**
 * Renders a text diff artifact.
 */
@customElement('milo-text-diff-artifact-page')
@consumer
export class TextDiffArtifactPageElement extends MobxLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeArtifactIdent()
  artifactIdent!: ArtifactIdentifier;

  @computed
  private get artifact$() {
    if (!this.store.services.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      this.store.services.resultDb.getArtifact({
        name: constructArtifactName(this.artifactIdent),
      }),
    );
  }
  @computed private get artifact() {
    return unwrapObservable(this.artifact$, null);
  }

  @computed
  private get content$() {
    if (!this.store.services.resultDb || !this.artifact) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      // TODO(crbug/1206109): use permanent raw artifact URL.
      fetch(
        urlSetSearchQueryParam(
          this.artifact.fetchUrl,
          'n',
          ARTIFACT_LENGTH_LIMIT,
        ),
      ).then((res) => res.text()),
    );
  }
  @computed private get content() {
    return unwrapObservable(this.content$, null);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    if (!this.artifact || !this.content) {
      return html`<div id="content" class="active-text">
        Loading <milo-dot-spinner></milo-dot-spinner>
      </div>`;
    }

    return html`
      <div id="details">
        <a href=${getRawArtifactURLPath(this.artifact.name)}
          >View Raw Content</a
        >
      </div>
      <div id="content">
        <link
          rel="stylesheet"
          type="text/css"
          href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css"
        />
        ${unsafeHTML(
          Diff2Html.html(this.content || '', {
            drawFileList: false,
            outputFormat: 'side-by-side',
          }),
        )}
      </div>
    `;
  });

  static styles = [
    commonStyles,
    css`
      #details {
        margin: 20px;
      }
      #content {
        position: relative;
        margin: 20px;
      }

      .d2h-code-linenumber {
        cursor: default;
      }
      .d2h-moved-tag {
        display: none;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-text-diff-artifact-page': Record<string, never>;
    }
  }
}

export function TextDiffArtifactPage() {
  return <milo-text-diff-artifact-page></milo-text-diff-artifact-page>;
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="text-diff">
    <TextDiffArtifactPage />
  </RecoverableErrorBoundary>
);
