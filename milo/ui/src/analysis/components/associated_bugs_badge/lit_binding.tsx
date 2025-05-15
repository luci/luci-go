// Copyright 2025 The LUCI Authors.
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
import { customElement } from 'lit/decorators.js';

import { OutputClusterEntry } from '@/analysis/types';
import { ReactLitElement } from '@/generic_libs/components/react_lit_element';

import {
  AssociatedBugsBadge,
  AssociatedBugsBadgeProps,
} from './associated_bugs_badge';

/** A custom element that wraps the AssociatedBugsBadge React component for inclusion in Lit Element components. */
@customElement('milo-associated-bugs-badge')
export class MiloAssociatedBugsBadge extends ReactLitElement {
  static get properties() {
    return {
      project: { type: String },
      clusters: { type: Array },
    };
  }

  private _project = '';
  get project() {
    return this._project;
  }
  set project(newVal: string) {
    if (this._project === newVal) {
      return;
    }
    const oldVal = this._project;
    this._project = newVal;
    this.requestUpdate('project', oldVal);
  }

  private _clusters: OutputClusterEntry[] = [];
  get clusters() {
    return this._clusters;
  }
  set clusters(newVal: OutputClusterEntry[]) {
    if (this._clusters === newVal) {
      return;
    }
    const oldVal = this._clusters;
    this._clusters = newVal;
    this.requestUpdate('clusters', oldVal);
  }

  private cache: EmotionCache | undefined = undefined;

  renderReact() {
    if (!this.cache) {
      this.cache = createCache({
        key: 'milo-associated-bugs-badge',
        container: this,
      });
    }

    return (
      <AssociatedBugsBadge
        project={this.project}
        clusters={this.clusters}
        cache={this.cache}
      />
    );
  }
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-associated-bugs-badge': AssociatedBugsBadgeProps;
    }
  }
}
