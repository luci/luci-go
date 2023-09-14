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

import '@material/mwc-icon';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { DateTime } from 'luxon';
import { autorun, computed, makeObservable, observable } from 'mobx';

import '@/common/components/timeline';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TimelineBlock } from '@/common/components/timeline';
import { Invocation } from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import {
  consumeInvocationState,
  InvocationStateInstance,
} from '@/common/store/invocation_state';
import { commonStyles } from '@/common/styles/stylesheets';
import { logging } from '@/common/tools/logging';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import { consumer } from '@/generic_libs/tools/lit_context';

const MARGIN = 20;
const MIN_GRAPH_WIDTH = 900;

function stripInvocationPrefix(invocationName: string): string {
  return invocationName.slice('invocations/'.length);
}

@customElement('milo-invocation-details-tab')
@consumer
export class InvocationDetailsTabElement extends MobxExtLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeInvocationState()
  invState!: InvocationStateInstance;

  @computed
  private get hasTags() {
    return (this.invState.invocation!.tags || []).length > 0;
  }

  @observable
  private graphWidth = MIN_GRAPH_WIDTH;

  @observable
  private numRequestsCompleted = 0;

  private includedInvocations: Invocation[] = [];

  constructor() {
    super();
    makeObservable(this);
    this.addDisposer(
      autorun(() => {
        try {
          if (
            !this.invState ||
            !this.invState.invocation ||
            !this.invState.invocation.includedInvocations
          ) {
            return;
          }
          let invs = this.invState.invocation.includedInvocations || [];
          // No more than 512 requests in flight at a time to prevent browsers cancelling them
          // TODO: implement a BatchGetInvocation call in ResultDB and remove this compensation.
          const delayedInvs = invs.slice(512);
          invs = invs.slice(0, 512);
          const invocationReceivedCallback = (invocation: Invocation) => {
            this.includedInvocations.push(invocation);
            // this.numRequestsCompleted += 1;
            this.batchRequestComplete();
            if (delayedInvs.length) {
              this.store.services.resultDb
                ?.getInvocation(
                  { name: delayedInvs.pop()! },
                  { skipUpdate: true },
                )
                .then(invocationReceivedCallback)
                .catch((e) => {
                  // TODO(mwarton): display the error to the user.
                  logging.error(e);
                });
            }
          };
          for (const invocationName of invs) {
            this.store.services.resultDb
              ?.getInvocation({ name: invocationName }, { skipUpdate: true })
              .then(invocationReceivedCallback)
              .catch((e) => {
                // TODO(mwarton): display the error to the user.
                logging.error(e);
              });
          }
        } catch (e) {
          // TODO(mwarton): display the error to the user.
          logging.error(e);
        }
      }),
    );
  }

  // requestsInBatch is NOT observable so we can batch updates to the observable numRequestsCompleted.
  private requestsInBatch = 0;
  // equivalent to this.numRequestsCompleted += 1, but batches all of the updates until the next idle period.
  // This ensures a render is only kicked off once the last one is finished, giving the minimum number of re-renders.
  batchRequestComplete() {
    if (this.requestsInBatch === 0) {
      window.requestIdleCallback(() => {
        this.numRequestsCompleted += this.requestsInBatch;
        this.requestsInBatch = 0;
      });
    }
    this.requestsInBatch += 1;
  }

  private now = DateTime.now();

  connectedCallback() {
    super.connectedCallback();
    this.now = DateTime.now();

    const syncWidth = () => {
      this.graphWidth = Math.max(
        window.innerWidth - 2 * MARGIN,
        MIN_GRAPH_WIDTH,
      );
    };
    window.addEventListener('resize', syncWidth);
    this.addDisposer(() => window.removeEventListener('resize', syncWidth));
    syncWidth();
  }

  protected render() {
    const invocation = this.invState.invocation;
    if (invocation === null) {
      return html``;
    }

    const blocks: TimelineBlock[] = this.includedInvocations.map((i) => ({
      text: stripInvocationPrefix(i.name),
      href: `/ui/inv/${stripInvocationPrefix(i.name)}/invocation-details`,
      start: DateTime.fromISO(i.createTime),
      end: i.finalizeTime ? DateTime.fromISO(i.finalizeTime) : undefined,
    }));
    blocks.sort((a, b) => {
      if (a.end && (!b.end || a.end < b.end)) {
        return -1;
      } else if (b.end && (!a.end || a.end > b.end)) {
        return 1;
      } else {
        // Invocations always have a create time, no need for undefined checks here.
        return a.start!.toMillis() - b.start!.toMillis();
      }
    });
    return html`
      <div>
        Create Time: ${new Date(invocation.createTime).toLocaleString()}
      </div>
      <div>
        Finalize Time: ${new Date(invocation.finalizeTime).toLocaleString()}
      </div>
      <div>Deadline: ${new Date(invocation.deadline).toLocaleDateString()}</div>
      <div style=${styleMap({ display: this.hasTags ? '' : 'none' })}>
        Tags:
        <table id="tag-table" border="0">
          ${invocation.tags?.map(
            (tag) => html`
              <tr>
                <td>${tag.key}:</td>
                <td>${tag.value}</td>
              </tr>
            `,
          )}
        </table>
      </div>
      <div id="included-invocations">
        ${invocation.includedInvocations?.length
          ? html`Included Invocations: (loaded ${this.numRequestsCompleted} of
              ${invocation.includedInvocations?.length})
              <milo-timeline
                .width=${this.graphWidth}
                .startTime=${DateTime.fromISO(invocation.createTime)}
                .endTime=${invocation.finalizeTime
                  ? DateTime.fromISO(invocation.finalizeTime)
                  : this.now}
                .blocks=${blocks}
              >
              </milo-timeline>`
          : 'Included Invocations: None'}
      </div>
    `;
  }

  static styles = [
    commonStyles,
    css`
      :host {
        display: block;
        padding: 10px 20px;
      }

      #included-invocations ul {
        list-style-type: none;
        margin-block-start: auto;
        margin-block-end: auto;
        padding-inline-start: 32px;
      }

      #tag-table {
        margin-left: 29px;
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-invocation-details-tab': Record<string, never>;
    }
  }
}

export function InvocationDetailsTab() {
  return <milo-invocation-details-tab />;
}

export function Component() {
  useTabId('invocation-details');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="invocation-details">
      <InvocationDetailsTab />
    </RecoverableErrorBoundary>
  );
}
