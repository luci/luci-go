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
import { MobxLitElement } from '@adobe/lit-mobx';
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { Duration } from 'luxon';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable, PENDING } from 'mobx-utils';

import '../../context/artifact_provider';
import '../associated_bugs_badge';
import '../expandable_entry';
import '../tags_entry';
import './image_diff_artifact';
import './link_artifact';
import './text_artifact';
import './text_diff_artifact';
import { TEST_STATUS_DISPLAY_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { reportRenderError } from '../../libs/error_handler';
import { unwrapObservable } from '../../libs/milo_mobx_utils';
import { displayCompactDuration, displayDuration, parseProtoDuration } from '../../libs/time_utils';
import { unwrapOrElse } from '../../libs/utils';
import { getRawArtifactUrl, router } from '../../routes';
import { Cluster, makeClusterLink } from '../../services/luci_analysis';
import { Artifact, ListArtifactsResponse, parseTestResultName, TestResult } from '../../services/resultdb';
import { consumeStore, StoreInstance } from '../../store';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';

/**
 * Renders an expandable entry of the given test result.
 */
@customElement('milo-result-entry')
@consumer
export class ResultEntryElement extends MobxLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref id = '';
  @observable.ref testResult!: TestResult;

  @observable.ref project = '';
  @observable.ref clusters: readonly Cluster[] = [];

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
  private get duration() {
    const durationStr = this.testResult.duration;
    if (!durationStr) {
      return null;
    }
    return Duration.fromMillis(parseProtoDuration(durationStr));
  }

  @computed
  private get parentInvId() {
    return parseTestResultName(this.testResult.name).invocationId;
  }

  @computed
  private get resultArtifacts$(): IPromiseBasedObservable<ListArtifactsResponse> {
    const resultdb = this.store.services.resultDb;
    if (!resultdb) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle pagination.
    return fromPromise(resultdb.listArtifacts({ parent: this.testResult.name }));
  }

  @computed private get resultArtifacts() {
    return unwrapOrElse(
      () => unwrapObservable(this.resultArtifacts$, {}).artifacts || [],
      // Optional resource, users may not have access to the artifacts.
      (e) => {
        if (!(e instanceof GrpcError && e.code === RpcCode.PERMISSION_DENIED)) {
          console.error(e);
        }
        return [];
      }
    );
  }

  @computed private get invArtifacts$() {
    const resultdb = this.store.services.resultDb;
    if (!resultdb) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle pagination.
    return fromPromise(resultdb.listArtifacts({ parent: 'invocations/' + this.parentInvId }));
  }

  @computed private get invArtifacts() {
    return unwrapOrElse(
      () => unwrapObservable(this.invArtifacts$, {}).artifacts || [],
      // Optional resource, users may not have access to the artifacts.
      (e) => {
        if (!(e instanceof GrpcError && e.code === RpcCode.PERMISSION_DENIED)) {
          console.error(e);
        }
        return [];
      }
    );
  }

  @computed private get artifactsMapping() {
    return new Map([
      ...this.resultArtifacts.map((obj) => [obj.artifactId, obj] as [string, Artifact]),
      ...this.invArtifacts.map((obj) => ['inv-level/' + obj.artifactId, obj] as [string, Artifact]),
    ]);
  }

  @computed private get stainlessLogArtifact() {
    return this.invArtifacts.find((a) => a.artifactId === 'stainless_logs');
  }

  @computed private get testhausLogArtifact() {
    return this.invArtifacts.find((a) => a.artifactId === 'testhaus_logs');
  }

  @computed private get textDiffArtifact() {
    return this.resultArtifacts.find((a) => a.artifactId === 'text_diff');
  }

  @computed private get imageDiffArtifactGroup() {
    return {
      expected: this.resultArtifacts.find((a) => a.artifactId === 'expected_image'),
      actual: this.resultArtifacts.find((a) => a.artifactId === 'actual_image'),
      diff: this.resultArtifacts.find((a) => a.artifactId === 'image_diff'),
    };
  }

  @computed private get clusterLink() {
    if (!this.project) {
      return null;
    }

    // There can be at most one failureReason cluster.
    const reasonCluster = this.clusters.filter((c) => c.clusterId.algorithm.startsWith('reason-'))?.[0];
    if (!reasonCluster) {
      return null;
    }

    return makeClusterLink(this.project, reasonCluster.clusterId);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private renderFailureReason() {
    const errMsg = this.testResult.failureReason?.primaryErrorMessage;
    if (!errMsg) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="none" .expanded=${true}>
        <span slot="header"
          >Failure
          Reason${this.clusterLink
            ? html` (<a href=${this.clusterLink} target="_blank" @click=${(e: Event) => e.stopImmediatePropagation()}
                  >similar failures</a
                >)`
            : ''}:
        </span>
        <pre id="failure-reason" class="info-block" slot="content">${errMsg}</pre>
      </milo-expandable-entry>
    `;
  }

  private renderSummaryHtml() {
    if (!this.testResult.summaryHtml) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="none" .expanded=${true}>
        <span slot="header">Summary:</span>
        <div slot="content">
          <div id="summary-html" class="info-block">
            <milo-artifact-provider
              .artifacts=${this.artifactsMapping}
              .finalized=${this.invArtifacts$.state !== PENDING && this.resultArtifacts$.state !== PENDING}
            >
              ${unsafeHTML(this.testResult.summaryHtml)}
            </milo-artifact-provider>
          </div>
          ${this.testhausLogArtifact &&
          html`
            <div class="summary-log-link">
              <milo-link-artifact .artifact=${this.testhausLogArtifact}></milo-link-artifact>
            </div>
          `}
          ${this.stainlessLogArtifact &&
          html`
            <div class="summary-log-link">
              <milo-link-artifact .artifact=${this.stainlessLogArtifact}></milo-link-artifact>
            </div>
          `}
        </div>
      </milo-expandable-entry>
    `;
  }

  private renderInvocationLevelArtifacts() {
    if (this.invArtifacts.length === 0) {
      return html``;
    }

    return html`
      <div id="inv-artifacts-header">
        From the parent inv <a href=${router.urlForName('invocation', { invocation_id: this.parentInvId })}></a>:
      </div>
      <ul>
        ${this.invArtifacts.map((artifact) => {
          if (artifact.contentType === 'text/x-uri') {
            return html` <li>
              <milo-link-artifact .artifact=${artifact}> </milo-link-artifact>
            </li>`;
          }
          return html`
            <li>
              <a href=${getRawArtifactUrl(artifact.name)} target="_blank">${artifact.artifactId}</a>
            </li>
          `;
        })}
      </ul>
    `;
  }

  private renderArtifacts() {
    const artifactCount = this.resultArtifacts.length + this.invArtifacts.length;
    if (artifactCount === 0) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="invisible">
        <span slot="header"> Artifacts: <span class="greyed-out">${artifactCount}</span> </span>
        <div slot="content">
          <ul>
            ${this.resultArtifacts.map(
              (artifact) => html`
                <li>
                  <a href=${getRawArtifactUrl(artifact.name)} target="_blank">${artifact.artifactId}</a>
                </li>
              `
            )}
          </ul>
          ${this.renderInvocationLevelArtifacts()}
        </div>
      </milo-expandable-entry>
    `;
  }

  private renderContent() {
    if (!this.shouldRenderContent) {
      return html``;
    }

    return html`
      ${this.renderFailureReason()}${this.renderSummaryHtml()}
      ${this.textDiffArtifact &&
      html` <milo-text-diff-artifact .artifact=${this.textDiffArtifact}> </milo-text-diff-artifact> `}
      ${this.imageDiffArtifactGroup.diff &&
      html`
        <milo-image-diff-artifact
          .expected=${this.imageDiffArtifactGroup.expected}
          .actual=${this.imageDiffArtifactGroup.actual}
          .diff=${this.imageDiffArtifactGroup.diff}
        >
        </milo-image-diff-artifact>
      `}
      ${this.renderArtifacts()}
      ${this.testResult.tags?.length ? html`<milo-tags-entry .tags=${this.testResult.tags}></milo-tags-entry>` : ''}
    `;
  }

  protected render = reportRenderError(this, () => {
    let duration = 'No duration';
    let compactDuration = 'N/A';
    let durationUnits = '';
    if (this.duration) {
      duration = displayDuration(this.duration);
      [compactDuration, durationUnits] = displayCompactDuration(this.duration);
    }
    return html`
      <milo-expandable-entry .expanded=${this.expanded} .onToggle=${(expanded: boolean) => (this.expanded = expanded)}>
        <span id="header" slot="header">
          <div class="duration ${durationUnits}" title=${duration}>${compactDuration}</div>
          run #${this.id}
          <span class=${this.testResult.expected ? 'expected' : 'unexpected'}>
            ${this.testResult.expected ? 'expectedly' : 'unexpectedly'}
            ${TEST_STATUS_DISPLAY_MAP[this.testResult.status]}
          </span>
          ${this.renderParentLink()}
          ${this.clusters.length && this.project
            ? html`<milo-associated-bugs-badge
                .project=${this.project}
                .clusters=${this.clusters}
              ></milo-associated-bugs-badge>`
            : ''}
        </span>
        <div slot="content">${this.renderContent()}</div>
      </milo-expandable-entry>
    `;
  });

  private renderParentLink() {
    const matchSwarming = this.parentInvId.match(/^task-(.+)-([0-9a-f]+)$/);
    if (matchSwarming) {
      return html`
        in task:
        <a
          href="https://${matchSwarming[1]}/task?id=${matchSwarming[2]}"
          target="_blank"
          @click=${(e: Event) => e.stopPropagation()}
        >
          ${matchSwarming[2]}
        </a>
      `;
    }

    // There's an alternative format for build invocation:
    // `build-${builderIdHash}-${buildNum}`.
    // We don't match that because:
    // 1. we can't get back the build link because the builderID is hashed, and
    // 2. typically those invocations are only used as wrapper invocations that
    // points to the `build-${buildId}` for the same build for speeding up
    // queries when buildId is not yet known to the client. We don't expect them
    // to be used here.
    const matchBuild = this.parentInvId.match(/^build-([0-9]+)$/);
    if (matchBuild) {
      return html`
        in build:
        <a href="/ui/b/${matchBuild[1]}" target="_blank" @click=${(e: Event) => e.stopPropagation()}>
          ${matchBuild[1]}
        </a>
      `;
    }

    return null;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      :host {
        display: block;
      }

      #header {
        display: inline-block;
        font-size: 14px;
        letter-spacing: 0.1px;
        font-weight: 500;
      }

      .info-block {
        background-color: var(--block-background-color);
        padding: 5px;
      }

      pre {
        margin: 0;
        font-size: 12px;
        white-space: pre-wrap;
        overflow-wrap: break-word;
      }

      #summary-html p:first-child {
        margin-top: 0;
      }
      #summary-html p:last-child {
        margin-bottom: 0;
      }

      .summary-log-link {
        padding-top: 5px;
      }

      ul {
        margin: 3px 0;
        padding-inline-start: 28px;
      }

      #inv-artifacts-header {
        margin-top: 12px;
      }

      milo-associated-bugs-badge {
        max-width: 300px;
        margin-left: 4px;
      }
    `,
  ];
}
