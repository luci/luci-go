// Copyright 2021 The LUCI Authors.
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
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise } from 'mobx-utils';
import { useEffect, useRef } from 'react';

import '@/analysis/components/lit_associated_bugs_badge';
import '@/generic_libs/components/expandable_entry';
import '@/generic_libs/components/copy_to_clipboard';
import '@/test_verdict/components/result_entry';
import { MAY_REQUIRE_SIGNIN, OPTIONAL_RESOURCE } from '@/common/common_tags';
import {
  VARIANT_STATUS_CLASS_MAP,
  VARIANT_STATUS_ICON_MAP,
} from '@/common/constants/legacy';
import { Cluster } from '@/common/services/luci_analysis';
import {
  RESULT_LIMIT,
  TestStatus,
  TestVariant,
} from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { colorClasses, commonStyles } from '@/common/styles/stylesheets';
import { logging } from '@/common/tools/logging';
import { getCodeSourceUrl } from '@/common/tools/url_utils';
import { ReactLitBridge } from '@/generic_libs/components/react_lit_element';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import {
  lazyRendering,
  RenderPlaceHolder,
} from '@/generic_libs/tools/observer_element';
import { attachTags, hasTags } from '@/generic_libs/tools/tag';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';
import { getSortedTestVariantDef } from '@/test_verdict/tools/utils';

import { consumeInvId, consumeProject, consumeTestTabUrl } from './context';

/**
 * Renders an expandable entry of the given test variant.
 */
@customElement('milo-test-variant-entry')
@lazyRendering
export class TestVariantEntryElement
  extends MobxLitElement
  implements RenderPlaceHolder
{
  @observable.ref @consumeStore() store!: StoreInstance;
  @observable.ref @consumeProject() project: string | undefined;
  @observable.ref @consumeInvId() invId: string | undefined;
  @observable.ref @consumeTestTabUrl() testTabUrl: string | undefined;

  @observable.ref variant!: TestVariant;
  @observable.ref columnGetters: Array<(v: TestVariant) => unknown> = [];
  @observable.ref historyUrl = '';

  @observable.ref private _expanded = false;
  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal: boolean) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  @computed
  private get testVerdictPageUrl() {
    if (!this.project || !this.invId || !this.invId.startsWith('build-')) {
      return '';
    }

    return `/ui/labs/p/${this.project}/inv/${
      this.invId
    }/test/${encodeURIComponent(this.variant.testId)}/variant/${
      this.variant.variantHash
    }`;
  }

  @computed
  private get shortName() {
    if (this.variant.testMetadata?.name) {
      return this.variant.testMetadata.name;
    }

    if (this.variant.testId.length <= 80) {
      // Name is already short enough as-is, no need to truncate.
      return this.variant.testId;
    }

    // Generate a good enough short name base on the test ID.
    const suffix = this.variant.testId.match(/^.*[./]([^./]*?.{60})$/);
    if (suffix) {
      return '...' + suffix[1];
    }
    return this.variant.testId;
  }

  @computed
  private get longName() {
    if (this.variant.testMetadata?.name) {
      return this.variant.testMetadata.name;
    }
    return this.variant.testId;
  }

  @computed
  private get clustersByResultId$() {
    if (!this.project || !this.store.services.clusters) {
      return fromPromise(Promise.race([]));
    }

    // We don't care about expected result nor unexpectedly passed/skipped
    // results. Filter them out.
    const results = this.variant.results?.filter(
      (r) =>
        !r.result.expected &&
        ![TestStatus.Pass, TestStatus.Skip].includes(r.result.status),
    );

    if (!results?.length) {
      return fromPromise(Promise.resolve([]));
    }

    return fromPromise(
      this.store.services.clusters
        .cluster(
          {
            project: this.project,
            testResults: results.map((r) => ({
              testId: this.variant.testId,
              failureReason: r.result.failureReason && {
                primaryErrorMessage:
                  r.result.failureReason?.primaryErrorMessage,
              },
            })),
          },
          {},
          { maxPendingMs: 1000 },
        )
        .catch((err) => {
          attachTags(err, OPTIONAL_RESOURCE);
          throw err;
        })
        .then((res) => {
          return res.clusteredTestResults.map(
            (ctr, i) =>
              [results[i].result.resultId, ctr.clusters] as readonly [
                string,
                readonly Cluster[],
              ],
          );
        }),
    );
  }

  @computed
  private get clustersByResultId(): ReadonlyArray<
    readonly [string, readonly Cluster[]]
  > {
    try {
      return unwrapObservable(this.clustersByResultId$, []);
    } catch (err) {
      if (!hasTags(err, MAY_REQUIRE_SIGNIN)) {
        logging.error(err);
      }
      // LUCI-Analysis integration should not break the rest of the component.
      return [];
    }
  }

  @computed
  private get clustersMap(): ReadonlyMap<string, readonly Cluster[]> {
    return new Map(this.clustersByResultId);
  }

  @computed
  private get uniqueClusters(): readonly Cluster[] {
    const clusters = this.clustersByResultId.flatMap(
      ([_, clusters]) => clusters,
    );
    const seen = new Set<string>();
    const uniqueClusters: Cluster[] = [];
    for (const cluster of clusters) {
      const key = cluster.clusterId.algorithm + '/' + cluster.clusterId.id;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      uniqueClusters.push(cluster);
    }
    return uniqueClusters;
  }

  @computed
  get selfLink() {
    if (!this.testTabUrl) {
      return null;
    }

    return urlSetSearchQueryParam(
      this.testTabUrl,
      'q',
      `ExactID:${this.variant.testId} VHash:${this.variant.variantHash}`,
    );
  }

  @computed
  private get sourceUrl() {
    const testLocation = this.variant?.testMetadata?.location;
    return testLocation
      ? getCodeSourceUrl(TestLocation.fromPartial(testLocation))
      : null;
  }

  @computed
  private get hasSingleChild() {
    return (
      (this.variant.results?.length ?? 0) +
        (this.variant.exonerations?.length ?? 0) ===
      1
    );
  }

  @computed
  private get variantDef() {
    return getSortedTestVariantDef(this.variant.variant?.def || {});
  }

  @computed
  private get expandedResultIndex() {
    // If there's only a single result, just expand it (even if it passed).
    if (this.hasSingleChild) {
      return 0;
    }
    // Otherwise expand the first failed result, or -1 if there aren't any.
    return this.variant.results?.findIndex((e) => !e.result.expected) ?? -1;
  }

  @computed private get columnValues() {
    return this.columnGetters.map((fn) => fn(this.variant));
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private renderBody() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div id="basic-info">
        ${this.historyUrl
          ? html`<a href=${this.historyUrl} target="_blank">history</a> |`
          : ''}
        ${this.sourceUrl
          ? html`<a href=${this.sourceUrl} target="_blank">source</a> |`
          : ''}
        <div id="test-id">
          <span class="greyed-out" title=${this.variant.testId}
            >ID: ${this.variant.testId}</span
          >
          <milo-copy-to-clipboard
            .textToCopy=${this.variant.testId}
            @click=${(e: Event) => e.stopPropagation()}
            title="copy test ID to clipboard"
          ></milo-copy-to-clipboard>
        </div>
        ${this.variantDef.length !== 0 ? '|' : ''}
        <span class="greyed-out">
          ${this.variantDef.map(
            ([k, v]) =>
              html` <span class="kv">
                <span class="kv-key">${k}</span>
                <span class="kv-value">${v}</span>
              </span>`,
          )}
        </span>
        ${this.testVerdictPageUrl
          ? html` |
              <a href=${this.testVerdictPageUrl} target="_blank">
                view in new test verdict page</a
              >`
          : ''}
        <span></span>
      </div>
      ${this.variant.results?.length === RESULT_LIMIT
        ? html`<div id="result-limit-warning">
            Only the first ${RESULT_LIMIT} results are displayed.
          </div>`
        : ''}
      ${repeat(
        this.variant.exonerations || [],
        (e) => e.exonerationId,
        (e) => html`
          <div class="explanation-html">
            ${unsafeHTML(
              e.explanationHtml ||
                'This test variant had unexpected results, but was exonerated (reason not provided).',
            )}
          </div>
        `,
      )}
      ${repeat(
        this.variant.results || [],
        (r) => r.result.resultId,
        (r, i) => html`
          <milo-result-entry
            .id=${i + 1}
            .testResult=${r.result}
            .clusters=${this.clustersMap.get(r.result.resultId) || []}
            .project=${this.project}
            .expanded=${i === this.expandedResultIndex}
          ></milo-result-entry>
        `,
      )}
    `;
  }

  renderPlaceHolder() {
    // Trigger the cluster RPC even when the entry is not rendered yet.
    // So we can batch more requests into one instead of waiting for new entries
    // to be progressively rendered.
    this.clustersByResultId$;
    return '';
  }

  protected render() {
    return html`
      <milo-expandable-entry
        .expanded=${this.expanded}
        .onToggle=${(expanded: boolean) => (this.expanded = expanded)}
      >
        <div id="header" slot="header">
          <mwc-icon class=${VARIANT_STATUS_CLASS_MAP[this.variant.status]}>
            ${VARIANT_STATUS_ICON_MAP[this.variant.status]}
          </mwc-icon>
          ${this.columnValues.map((v) => html`<div title=${v}>${v}</div>`)}
          <div id="test-name">
            <span title=${this.longName}>${this.shortName}</span>
            ${this.uniqueClusters.length && !this.expanded
              ? html`<milo-associated-bugs-badge
                  .project=${this.project}
                  .clusters=${this.uniqueClusters}
                  @click=${(e: Event) => e.stopPropagation()}
                ></milo-associated-bugs-badge>`
              : ''}
            <milo-copy-to-clipboard
              .textToCopy=${this.longName}
              @click=${(e: Event) => e.stopPropagation()}
              title="copy test name to clipboard"
            ></milo-copy-to-clipboard>
            ${this.selfLink
              ? html` <milo-copy-to-clipboard
                  id="link-copy-button"
                  .textToCopy=${this.selfLink}
                  @click=${(e: Event) => e.stopPropagation()}
                  title="copy link to the test"
                >
                  <mwc-icon slot="copy-icon">link</mwc-icon>
                </milo-copy-to-clipboard>`
              : ''}
          </div>
        </div>
        <div id="body" slot="content">${this.renderBody()}</div>
      </milo-expandable-entry>
    `;
  }

  private static _styles = [
    commonStyles,
    colorClasses,
    css`
      :host {
        display: block;
        min-height: 24px;
      }

      #header {
        display: grid;
        grid-template-columns: var(--tvt-columns);
        grid-gap: 5px;
        font-size: 16px;
        line-height: 24px;
      }
      #header > * {
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #test-name {
        display: flex;
        font-size: 16px;
        line-height: 24px;
      }
      #test-name > span {
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #body {
        overflow: hidden;
      }

      #basic-info {
        font-weight: 500;
        line-height: 24px;
        margin-left: 5px;
      }

      #test-id {
        display: inline-flex;
        max-width: 300px;
        overflow: hidden;
        white-space: nowrap;
      }
      #test-id > span {
        display: inline-block;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      .kv-key::after {
        content: ':';
      }
      .kv-value::after {
        content: ',';
      }
      .kv:last-child > .kv-value::after {
        content: '';
      }
      #def-table {
        margin-left: 29px;
      }

      .greyed-out {
        color: var(--greyed-out-text-color);
      }

      .explanation-html {
        background-color: var(--block-background-color);
        padding: 5px;
      }

      milo-copy-to-clipboard {
        flex: 0 0 16px;
        margin-left: 2px;
        display: none;
      }
      :hover > milo-copy-to-clipboard {
        display: inline-block;
      }

      #result-limit-warning {
        padding: 5px;
        background-color: var(--warning-color);
        font-weight: 500;
      }

      milo-associated-bugs-badge {
        max-width: 150px;
        flex-shrink: 0;
        margin-left: 4px;
      }
    `,
  ];
  public static get styles() {
    return TestVariantEntryElement._styles;
  }
  public static set styles(value) {
    TestVariantEntryElement._styles = value;
  }
}

export interface TestVariantEntryProps {
  readonly variant: TestVariant;
  readonly columnGetters: Array<(v: TestVariant) => unknown>;
  readonly historyUrl?: string;
}

export function TestVariantEntry({
  variant,
  columnGetters,
  historyUrl,
}: TestVariantEntryProps) {
  const container = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    // This will never happen. But useful for type checking.
    if (!container.current) {
      return;
    }
    render(
      html`<milo-test-variant-entry
        .variant=${variant}
        .columnGetters=${columnGetters}
        .historyUrl=${historyUrl}
      ></milo-test-variant-entry>`,
      container.current,
    );
  }, [variant, columnGetters, historyUrl]);

  return (
    <ReactLitBridge>
      <div ref={container} />
    </ReactLitBridge>
  );
}
