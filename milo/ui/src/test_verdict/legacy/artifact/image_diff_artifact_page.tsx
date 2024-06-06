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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';

import '@/common/components/image_diff_viewer';
import '@/common/components/status_bar';
import '@/generic_libs/components/dot_spinner';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  ArtifactIdentifier,
  constructArtifactName,
} from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { commonStyles } from '@/common/styles/stylesheets';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';

import { consumeArtifactIdent } from './artifact_page_layout';

/**
 * Renders an image diff artifact set, including expected image, actual image
 * and image diff.
 */
// TODO(weiweilin): improve error handling.
@customElement('milo-image-diff-artifact-page')
@consumer
export class ImageDiffArtifactPageElement extends MobxLitElement {
  static get properties() {
    return {
      expectedArtifactId: {
        type: String,
      },
      actualArtifactId: {
        type: String,
      },
    };
  }

  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeArtifactIdent()
  artifactIdent!: ArtifactIdentifier;

  @observable.ref _expectedArtifactId!: string;
  @computed get expectedArtifactId() {
    return this._expectedArtifactId;
  }
  set expectedArtifactId(newVal: string) {
    this._expectedArtifactId = newVal;
  }

  @observable.ref _actualArtifactId!: string;
  @computed get actualArtifactId() {
    return this._actualArtifactId;
  }
  set actualArtifactId(newVal: string) {
    this._actualArtifactId = newVal;
  }

  @computed private get diffArtifactName() {
    return constructArtifactName({ ...this.artifactIdent });
  }
  @computed private get expectedArtifactName() {
    return constructArtifactName({
      ...this.artifactIdent,
      artifactId: this.expectedArtifactId,
    });
  }
  @computed private get actualArtifactName() {
    return constructArtifactName({
      ...this.artifactIdent,
      artifactId: this.actualArtifactId,
    });
  }

  @computed
  private get diffArtifact$() {
    if (!this.store.services.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      this.store.services.resultDb.getArtifact({ name: this.diffArtifactName }),
    );
  }
  @computed private get diffArtifact() {
    return unwrapObservable(this.diffArtifact$, null);
  }

  @computed
  private get expectedArtifact$() {
    if (!this.store.services.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      this.store.services.resultDb.getArtifact({
        name: this.expectedArtifactName,
      }),
    );
  }
  @computed private get expectedArtifact() {
    return unwrapObservable(this.expectedArtifact$, null);
  }

  @computed
  private get actualArtifact$() {
    if (!this.store.services.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(
      this.store.services.resultDb.getArtifact({
        name: this.actualArtifactName,
      }),
    );
  }
  @computed private get actualArtifact() {
    return unwrapObservable(this.actualArtifact$, null);
  }

  @computed get isLoading() {
    return !this.expectedArtifact || !this.actualArtifact || !this.diffArtifact;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    if (this.isLoading) {
      return html`<div id="loading-spinner" class="active-text">
        Loading <milo-dot-spinner></milo-dot-spinner>
      </div>`;
    }

    return html`
      <milo-image-diff-viewer
        .expected=${this.expectedArtifact}
        .actual=${this.actualArtifact}
        .diff=${this.diffArtifact}
      >
      </milo-image-diff-viewer>
    `;
  });

  static styles = [
    commonStyles,
    css`
      :host {
        display: block;
      }

      #loading-spinner {
        margin: 20px;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-image-diff-artifact-page': {
        expectedArtifactId: string;
        actualArtifactId: string;
      };
    }
  }
}

export function ImageDiffArtifactPage() {
  const [search] = useSyncedSearchParams();

  const expectedArtifactId = search.get('expectedArtifactId');
  if (expectedArtifactId === null) {
    throw new Error(
      'expectedArtifactId must be provided via the search params',
    );
  }

  const actualArtifactId = search.get('actualArtifactId');
  if (actualArtifactId === null) {
    throw new Error('actualArtifactId must be provided via the search params');
  }

  return (
    <milo-image-diff-artifact-page
      expectedArtifactId={expectedArtifactId}
      actualArtifactId={actualArtifactId}
    ></milo-image-diff-artifact-page>
  );
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="image-diff">
      <ImageDiffArtifactPage />
    </RecoverableErrorBoundary>
  );
}
