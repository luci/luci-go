// Copyright 2023 The LUCI Authors.
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

import '@material/mwc-icon';
import { html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { ReactNode } from 'react';

import { consumeStore, StoreInstance } from '@/common/store';
import { provideInvocationState } from '@/common/store/invocation_state';
import { getInvURLPath } from '@/common/tools/url_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer, provider } from '@/generic_libs/tools/lit_context';

import {
  provideProject,
  provideTestTabUrl,
} from '../test_results_tab/test_variants_table/context';

/**
 * Provides context to lit components in an invocation page.
 */
@customElement('milo-inv-lit-env-provider')
@provider
@consumer
export class InvLitEnvProviderElement extends MobxExtLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @provideInvocationState()
  @computed
  get invState() {
    return this.store.invocationPage.invocation;
  }

  @provideProject({ global: true })
  @computed
  get project() {
    return this.store.invocationPage.invocation.project ?? undefined;
  }

  @provideTestTabUrl({ global: true })
  @computed
  get testTabUrl() {
    if (!this.store.invocationPage.invocationId) {
      return undefined;
    }
    return (
      getInvURLPath(this.store.invocationPage.invocationId) + '/test-results'
    );
  }

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => [this.invState],
        ([invState]) => {
          // Emulate @property() update.
          this.updated(new Map([['invState', invState]]));
        },
        { fireImmediately: true },
      ),
    );

    this.addDisposer(
      reaction(
        () => this.project,
        (project) => {
          // Emulate @property() update.
          this.updated(new Map([['project', project]]));
        },
        { fireImmediately: true },
      ),
    );

    this.addDisposer(
      reaction(
        () => this.testTabUrl,
        (testTabUrl) => {
          // Emulate @property() update.
          this.updated(new Map([['testTabUrl', testTabUrl]]));
        },
        { fireImmediately: true },
      ),
    );
  }

  protected render() {
    return html`<slot></slot>`;
  }
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-inv-lit-env-provider': {
        children: ReactNode;
      };
    }
  }
}

export interface InvLitEnvProviderProps {
  readonly children: React.ReactNode;
}

export function InvLitEnvProvider({ children }: InvLitEnvProviderProps) {
  return <milo-inv-lit-env-provider>{children}</milo-inv-lit-env-provider>;
}
