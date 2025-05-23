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

import '@/analysis/components/associated_bugs_badge';
import '@/common/components/sanitized_html';
import '@/common/components/tags_entry';
import '@/generic_libs/components/expandable_entry';
import '@/test_verdict/components/artifact_tags';

import './image_diff_artifact';
import './link_artifact';
import './text_diff_artifact';

import { MobxLitElement } from '@adobe/lit-mobx';
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { makeClusterLink } from '@/analysis/tools/utils';
import {
  TEST_STATUS_V2_CLASS_MAP,
  TEST_STATUS_V2_DISPLAY_MAP,
  WEB_TEST_STATUS_DISPLAY_MAP,
} from '@/common/constants/legacy';
import { Cluster } from '@/common/services/luci_analysis';
import {
  Artifact,
  FailureReason_Kind,
  ListArtifactsResponse,
  SkippedReason_Kind,
  TestResult,
  TestResult_Status,
} from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { colorClasses, commonStyles } from '@/common/styles/stylesheets';
import { parseInvId } from '@/common/tools/invocation_utils';
import { logging } from '@/common/tools/logging';
import {
  displayCompactDuration,
  displayDuration,
  parseProtoDurationStr,
} from '@/common/tools/time_utils';
import {
  getInvURLPath,
  getRawArtifactURLPath,
  getSwarmingTaskURL,
} from '@/common/tools/url_utils';
import { reportRenderError } from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';
import { unwrapObservable } from '@/generic_libs/tools/mobx_utils';
import { unwrapOrElse } from '@/generic_libs/tools/utils';
import { parseTestResultName } from '@/test_verdict/tools/utils';

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
    return parseProtoDurationStr(durationStr);
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
    return fromPromise(
      resultdb.listArtifacts({ parent: this.testResult.name }),
    );
  }

  @computed private get resultArtifacts() {
    return unwrapOrElse(
      () => unwrapObservable(this.resultArtifacts$, {}).artifacts || [],
      // Optional resource, users may not have access to the artifacts.
      (e) => {
        if (!(e instanceof GrpcError && e.code === RpcCode.PERMISSION_DENIED)) {
          logging.error(e);
        }
        return [];
      },
    );
  }

  @computed private get invArtifacts$() {
    const resultdb = this.store.services.resultDb;
    if (!resultdb) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle pagination.
    return fromPromise(
      resultdb.listArtifacts({ parent: 'invocations/' + this.parentInvId }),
    );
  }

  @computed private get invArtifacts() {
    return unwrapOrElse(
      () => unwrapObservable(this.invArtifacts$, {}).artifacts || [],
      // Optional resource, users may not have access to the artifacts.
      (e) => {
        if (!(e instanceof GrpcError && e.code === RpcCode.PERMISSION_DENIED)) {
          logging.error(e);
        }
        return [];
      },
    );
  }

  @computed private get stainlessLogArtifact() {
    return this.invArtifacts.find((a) => a.artifactId === 'stainless_logs');
  }

  @computed private get testhausLogArtifact() {
    // Check for Testhaus logs at the test result level first.
    const log = this.resultArtifacts.find(
      (a) => a.artifactId === 'testhaus_logs',
    );
    if (log) {
      return log;
    }

    // Now check at the parent invocation level.
    return this.invArtifacts.find((a) => a.artifactId === 'testhaus_logs');
  }

  @computed private get textDiffArtifact() {
    return this.resultArtifacts.find((a) => a.artifactId === 'text_diff');
  }

  @computed private get imageDiffArtifactGroup() {
    return {
      expected: this.resultArtifacts.find(
        (a) => a.artifactId === 'expected_image',
      ),
      actual: this.resultArtifacts.find((a) => a.artifactId === 'actual_image'),
      diff: this.resultArtifacts.find((a) => a.artifactId === 'image_diff'),
    };
  }

  @computed private get clusterLink() {
    if (!this.project) {
      return null;
    }

    // There can be at most one failureReason cluster.
    const reasonCluster = this.clusters.filter((c) =>
      c.clusterId.algorithm.startsWith('reason-'),
    )?.[0];
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
            ? html` (<a
                  href=${this.clusterLink}
                  target="_blank"
                  @click=${(e: Event) => e.stopImmediatePropagation()}
                  >similar failures</a
                >)`
            : ''}:
        </span>
        <pre id="failure-reason" class="info-block" slot="content">
${errMsg}</pre
        >
      </milo-expandable-entry>
    `;
  }

  private renderSkippedReason() {
    const reasonMsg = this.testResult.skippedReason?.reasonMessage;
    if (!reasonMsg) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="none" .expanded=${true}>
        <span slot="header">Skipped Reason: </span>
        ${reasonMsg &&
        html`<pre id="skipped-reason" class="info-block" slot="content">
${reasonMsg}</pre
        >`}
      </milo-expandable-entry>
    `;
  }

  private renderLogLinkArtifacts() {
    if (this.testhausLogArtifact || this.stainlessLogArtifact) {
      let testhausLink = null;
      if (this.testhausLogArtifact) {
        testhausLink = html`<milo-link-artifact
          .artifact=${this.testhausLogArtifact}
          .label="Testhaus"
        ></milo-link-artifact>`;
      }
      let delimiter = null;
      if (this.testhausLogArtifact && this.stainlessLogArtifact) {
        delimiter = ', ';
      }
      let stainlessLink = null;
      if (this.stainlessLogArtifact) {
        stainlessLink = html`<milo-link-artifact
          .artifact=${this.stainlessLogArtifact}
          .label="Stainless"
        ></milo-link-artifact>`;
      }

      return html`
        <div class="summary-log-link">
          View logs in: ${testhausLink}${delimiter}${stainlessLink}
        </div>
      `;
    }

    return null;
  }

  private renderSummaryHtml() {
    if (!this.testResult.summaryHtml) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="none" .expanded=${true}>
        <span slot="header">Summary:</span>
        <div slot="content">
          <milo-artifact-tag-context-provider
            result-name=${this.testResult.name}
          >
            <div id="summary-html" class="info-block">
              <milo-sanitized-html
                html=${this.testResult.summaryHtml}
              ></milo-sanitized-html>
            </div>
          </milo-artifact-tag-context-provider>
          ${this.renderLogLinkArtifacts()}
        </div>
      </milo-expandable-entry>
    `;
  }

  private renderArtifactLink(artifact: Artifact) {
    if (artifact.contentType === 'text/x-uri') {
      return html`<milo-link-artifact
        .artifact=${artifact}
      ></milo-link-artifact>`;
    }
    return html`<a href=${getRawArtifactURLPath(artifact.name)} target="_blank"
      >${artifact.artifactId}</a
    >`;
  }

  private renderInvocationLevelArtifacts() {
    if (this.invArtifacts.length === 0) {
      return html``;
    }

    return html`
      <div id="inv-artifacts-header">
        From the
        <a href=${getInvURLPath(this.parentInvId)}>parent invocation</a>:
      </div>
      <ul>
        ${this.invArtifacts.map(
          (artifact) => html` <li>${this.renderArtifactLink(artifact)}</li> `,
        )}
      </ul>
    `;
  }

  private renderArtifacts() {
    const artifactCount =
      this.resultArtifacts.length + this.invArtifacts.length;
    if (artifactCount === 0) {
      return html``;
    }

    return html`
      <milo-expandable-entry .contentRuler="invisible">
        <span slot="header">
          Artifacts: <span class="greyed-out">${artifactCount}</span>
        </span>
        <div slot="content">
          <ul>
            ${this.resultArtifacts.map(
              (artifact) => html`
                <li>${this.renderArtifactLink(artifact)}</li>
              `,
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
      ${this.renderFailureReason()}${this.renderSkippedReason()}
      ${this.renderSummaryHtml()}
      ${this.textDiffArtifact &&
      html`
        <milo-text-diff-artifact .artifact=${this.textDiffArtifact}>
        </milo-text-diff-artifact>
      `}
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
      ${this.testResult.tags?.length
        ? html`<milo-tags-entry
            .tags=${this.testResult.tags}
          ></milo-tags-entry>`
        : ''}
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
      <milo-expandable-entry
        .expanded=${this.expanded}
        .onToggle=${(expanded: boolean) => (this.expanded = expanded)}
      >
        <span id="header" slot="header">
          <div class="duration ${durationUnits}" title=${duration}>
            ${compactDuration}
          </div>
          result #${this.id} ${this.renderStatus()} ${this.renderParentLink()}
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

  private renderStatus() {
    const requiresLeadingWas =
      this.testResult.statusV2 === TestResult_Status.PRECLUDED ||
      this.testResult.statusV2 === TestResult_Status.SKIPPED;
    const webTest = this.testResult.frameworkExtensions?.webTest;
    const failureKind =
      this.testResult.failureReason?.kind ||
      FailureReason_Kind.KIND_UNSPECIFIED;
    const skippedKind =
      this.testResult.skippedReason?.kind ||
      SkippedReason_Kind.KIND_UNSPECIFIED;

    let statusDetail: string = '';
    if (webTest) {
      statusDetail = `${webTest.isExpected ? 'expectedly' : 'unexpectedly'} ${WEB_TEST_STATUS_DISPLAY_MAP[webTest.status]}`;
    } else if (
      this.testResult.statusV2 === TestResult_Status.FAILED &&
      failureKind !== FailureReason_Kind.KIND_UNSPECIFIED
    ) {
      switch (failureKind) {
        case FailureReason_Kind.CRASH:
          statusDetail = 'crashed';
          break;
        case FailureReason_Kind.TIMEOUT:
          statusDetail = 'timed out';
          break;
        case FailureReason_Kind.ORDINARY:
          // No detail to show.
          break;
      }
    } else if (
      this.testResult.statusV2 === TestResult_Status.SKIPPED &&
      skippedKind !== SkippedReason_Kind.KIND_UNSPECIFIED
    ) {
      switch (skippedKind) {
        case SkippedReason_Kind.DEMOTED:
          statusDetail = 'demoted';
          break;
        case SkippedReason_Kind.DISABLED_AT_DECLARATION:
          statusDetail = 'disabled at declaration';
          break;
        case SkippedReason_Kind.SKIPPED_BY_TEST_BODY:
          statusDetail = 'by test body';
          break;
        case SkippedReason_Kind.OTHER:
          // No status detail to show, but the failure reason section will contain information.
          break;
      }
    }

    return html`${requiresLeadingWas ? 'was ' : ''}
      <span class=${TEST_STATUS_V2_CLASS_MAP[this.testResult.statusV2]}>
        ${TEST_STATUS_V2_DISPLAY_MAP[this.testResult.statusV2]}
        ${statusDetail ? html` (${statusDetail})` : null}
      </span>`;
  }

  private renderParentLink() {
    const parsedInvId = parseInvId(this.parentInvId);
    if (parsedInvId.type === 'swarming-task') {
      return html`
        in task:
        <a
          href="${getSwarmingTaskURL(
            parsedInvId.swarmingHost,
            parsedInvId.taskId,
          )}"
          target="_blank"
          @click=${(e: Event) => e.stopPropagation()}
        >
          ${parsedInvId.taskId}
        </a>
      `;
    }

    if (parsedInvId.type === 'build') {
      return html`
        in build:
        <a
          href="/ui/b/${parsedInvId.buildId}"
          target="_blank"
          @click=${(e: Event) => e.stopPropagation()}
        >
          ${parsedInvId.buildId}
        </a>
      `;
    }

    return null;
  }

  static styles = [
    commonStyles,
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
