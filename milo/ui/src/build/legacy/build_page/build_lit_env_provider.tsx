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

import { css as litCss, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { ReactNode } from 'react';

import { OPTIONAL_RESOURCE } from '@/common/common_tags';
import { LoadTestVariantsError } from '@/common/models/test_loader';
import { consumeStore, StoreInstance } from '@/common/store';
import {
  provideInvocationState,
  QueryInvocationError,
} from '@/common/store/invocation_state';
import { getBuildURLPath } from '@/common/tools/url_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import {
  errorHandler,
  forwardWithoutMsg,
} from '@/generic_libs/tools/error_handler';
import { consumer, provider } from '@/generic_libs/tools/lit_context';
import { attachTags } from '@/generic_libs/tools/tag';
import {
  provideInvId,
  provideProject,
  provideTestTabUrl,
} from '@/test_verdict/legacy/test_results_tab/test_variants_table/context';

function retryWithoutComputedInvId(
  err: ErrorEvent,
  ele: BuildLitEnvProviderElement,
) {
  let recovered = false;
  if (err.error instanceof LoadTestVariantsError) {
    // Ignore request using the old invocation ID.
    if (
      !err.error.req.invocations.includes(
        `invocations/${ele.store.buildPage.invocationId}`,
      )
    ) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.store.buildPage.useComputedInvId && !err.error.req.pageToken) {
      ele.store.buildPage.setUseComputedInvId(false);
      recovered = true;
    }
  } else if (err.error instanceof QueryInvocationError) {
    // Ignore request using the old invocation ID.
    if (err.error.invId !== ele.store.buildPage.invocationId) {
      recovered = true;
    }

    // Old builds don't support computed invocation ID.
    // Disable it and try again.
    if (ele.store.buildPage.useComputedInvId) {
      ele.store.buildPage.setUseComputedInvId(false);
      recovered = true;
    }
  }

  if (recovered) {
    err.stopImmediatePropagation();
    err.preventDefault();
    return false;
  }

  attachTags(err.error, OPTIONAL_RESOURCE);
  return forwardWithoutMsg(err, ele);
}

/**
 * Provides context and error handling to lit components in a build page.
 */
@customElement('milo-build-lit-env-provider')
@errorHandler(retryWithoutComputedInvId)
@provider
@consumer
export class BuildLitEnvProviderElement extends MobxExtLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @provideInvocationState({ global: true })
  @computed
  get invState() {
    return this.store.buildPage.invocation;
  }

  @provideProject({ global: true })
  @computed
  get project() {
    return this.store.buildPage.build?.data.builder.project;
  }

  @provideInvId({ global: true })
  @computed
  get invId() {
    const invName =
      this.store.buildPage.build?.data.infra?.resultdb?.invocation;
    return invName ? invName.slice('invocations/'.length) : undefined;
  }

  @provideTestTabUrl({ global: true })
  @computed
  get testTabUrl() {
    if (
      !this.store.buildPage.builderIdParam ||
      !this.store.buildPage.buildNumOrIdParam
    ) {
      return undefined;
    }
    return (
      getBuildURLPath(
        this.store.buildPage.builderIdParam,
        this.store.buildPage.buildNumOrIdParam,
      ) + '/test-results'
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
        () => this.invState,
        (invState) => {
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
        () => this.invId,
        (invId) => {
          // Emulate @property() update.
          this.updated(new Map([['invId', invId]]));
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

  static styles = [
    litCss`
      #build-not-found-error {
        background-color: var(--warning-color);
        font-weight: 500;
        padding: 5px;
        margin: 8px 16px;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-build-lit-env-provider': {
        children: ReactNode;
      };
    }
  }
}

export interface BuildLitEnvProviderProps {
  readonly children: React.ReactNode;
}

export function BuildLitEnvProvider({ children }: BuildLitEnvProviderProps) {
  return <milo-build-lit-env-provider>{children}</milo-build-lit-env-provider>;
}
