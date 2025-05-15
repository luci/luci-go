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

import { html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable, reaction } from 'mobx';
import { ReactNode, useEffect, useRef, useState } from 'react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { MAY_REQUIRE_SIGNIN, OPTIONAL_RESOURCE } from '@/common/common_tags';
import { provideStore, StoreInstance, useStore } from '@/common/store';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import {
  errorHandler,
  handleLocally,
} from '@/generic_libs/tools/error_handler';
import { provider } from '@/generic_libs/tools/lit_context';
import {
  ProgressiveNotifier,
  provideNotifier,
} from '@/generic_libs/tools/observer_element';
import { hasTags } from '@/generic_libs/tools/tag';

function redirectToLogin(err: ErrorEvent, ele: LitEnvProviderElement) {
  if (
    ele.store.authState.identity === ANONYMOUS_IDENTITY &&
    hasTags(err.error, MAY_REQUIRE_SIGNIN) &&
    !hasTags(err.error, OPTIONAL_RESOURCE)
  ) {
    window.location.href = `/ui/login?${new URLSearchParams([
      ['redirect', location.pathname + location.search + location.hash],
    ])}`;
    return false;
  }
  return handleLocally(err, ele);
}

/**
 * Provides context and error handling to lit components.
 */
@customElement('milo-lit-env-provider')
@errorHandler(redirectToLogin)
@provider
export class LitEnvProviderElement extends MobxExtLitElement {
  @observable.ref
  @provideStore({ global: true })
  store!: StoreInstance;

  @provideNotifier({ global: true }) readonly notifier =
    new ProgressiveNotifier({
      // Ensures that everything above the current scroll view is rendered.
      // This reduces page shifting due to incorrect height estimate.
      rootMargin: '1000000px 0px 0px 0px',
    });

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.store,
        (store) => {
          // Emulate @property() update.
          this.updated(new Map([['store', store]]));
        },
        { fireImmediately: true },
      ),
    );
  }

  protected render() {
    return html`<slot></slot>`;
  }
}

declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-lit-env-provider': {
        children: ReactNode;
        ref: React.MutableRefObject<LitEnvProviderElement | null>;
      };
    }
  }
}

export interface LitEnvProviderProps {
  readonly children: React.ReactNode;
}

export function LitEnvProvider({ children }: LitEnvProviderProps) {
  const store = useStore();
  const envProviderRef = useRef<LitEnvProviderElement>(null);
  const [initialized, setInitialized] = useState(false);

  // Keep React2LitAdaptorElement.store up to date.
  useEffect(() => {
    // This will never happen. But useful for type checking.
    if (!envProviderRef.current) {
      return;
    }
    envProviderRef.current.store = store;
    setInitialized(true);
  }, [store]);

  return (
    <milo-lit-env-provider ref={envProviderRef}>
      {/* Only render the children after the store is populated. */}
      {initialized ? children : null}
    </milo-lit-env-provider>
  );
}
