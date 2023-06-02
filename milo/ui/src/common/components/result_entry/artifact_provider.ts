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

import { html, PropertyValues } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable, reaction } from 'mobx';

import { MiloBaseElement } from '@/common/components/milo_base';
import { createContextLink, provider } from '@/common/libs/context';
import { Artifact } from '@/common/services/resultdb';

export const [provideArtifacts, consumeArtifacts] =
  createContextLink<Map<string, Artifact>>();
export const [provideArtifactsFinalized, consumeArtifactsFinalized] =
  createContextLink<boolean>();

/**
 * Provides artifacts information.
 */
@customElement('milo-artifact-provider')
@provider
export class ArtifactProvider extends MiloBaseElement {
  @observable.ref
  @provideArtifacts()
  artifacts!: Map<string, Artifact>;

  @observable.ref
  @provideArtifactsFinalized()
  finalized = false;

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback(): void {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.artifacts,
        (artifacts) => {
          // Emulate @property() update.
          this.updated(new Map([['artifacts', artifacts]]));
        },
        { fireImmediately: true }
      )
    );
    this.addDisposer(
      reaction(
        () => this.finalized,
        (finalized) => {
          // Emulate @property() update.
          this.updated(new Map([['finalized', finalized]]));
        },
        { fireImmediately: true }
      )
    );
  }

  protected updated(changedProperties: PropertyValues) {
    super.updated(changedProperties);
  }

  protected render() {
    return html` <slot></slot> `;
  }
}
