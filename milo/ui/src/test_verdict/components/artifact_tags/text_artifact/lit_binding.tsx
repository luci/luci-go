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
import { customElement } from 'lit/decorators.js';

import { ReactLitElement } from '@/generic_libs/components/react_lit_element';
import { consumer } from '@/generic_libs/tools/lit_context';

import { ArtifactContextProvider } from '../context';
import { consumeResultName } from '../lit_context';

import { TextArtifact } from './text_artifact';

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
