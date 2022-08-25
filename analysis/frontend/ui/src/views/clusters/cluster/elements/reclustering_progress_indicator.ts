/* eslint-disable @typescript-eslint/indent */
// Copyright 2022 The LUCI Authors.
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

import '@material/mwc-button';
import '@material/mwc-circular-progress';

import {
    css,
    customElement,
    html,
    LitElement,
    property,
    state,
    TemplateResult
} from 'lit-element';
import { DateTime } from 'luxon';

import { ReclusteringProgress } from '../../../../services/cluster';

import {
    fetchProgress,
    progressToLatestAlgorithms,
    progressToLatestConfig,
    progressToRulesVersion,
} from '../../../../tools/progress_tools';

/**
 * ReclusteringProgressIndicator displays the progress Weetbix is making
 * re-clustering test results to reflect current algorithms and
 * the current rule.
 */
@customElement('reclustering-progress-indicator')
export class ReclusteringProgressIndicator extends LitElement {
    @property()
    project = '';

    @property({ type: Boolean })
    // Whether the cluster for which the indicator is being shown is
    // defined by a failure association rule.
    hasRule: boolean | undefined;

    @property()
    // The last updated time of the rule predicate which defines the
    // cluster (if any).
    // This should be set if hasRule is true.
    rulePredicateLastUpdated: string | undefined;

    @state()
    progress: ReclusteringProgress | undefined;

    @state()
    lastRefreshed: DateTime | undefined;

    @state()
    // Whether the indicator should be displayed. If re-clustering
    // is not complete, this will be set to true. It will only ever
    // be set to false if re-clustering is complete and the user
    // reloads cluster analysis.
    show = false;

    // The last progress shown on the UI.
    progressPerMille = 1000;

    // The ID returned by window.setInterval. Used to manage the timer
    // used to periodically poll for status updates.
    interval: number | undefined;

    connectedCallback() {
        super.connectedCallback();

        this.interval = window.setInterval(() => {
            this.timerTick();
        }, 5000);

        this.show = false;

        this.fetch();
    }

    disconnectedCallback() {
        super.disconnectedCallback();
        if (this.interval !== undefined) {
            window.clearInterval(this.interval);
        }
    }

    // tickerTick is called periodically. Its purpose is to obtain the
    // latest re-clustering progress if progress is not complete.
    timerTick() {
        // Only fetch updates if the indicator is being shown. This avoids
        // creating server load for no appreciable UX improvement.
        if (document.visibilityState == 'visible' &&
            this.progressPerMille < 1000) {
            this.fetch();
        }
    }

    render() {
        if (this.progress === undefined ||
            (this.hasRule && !this.rulePredicateLastUpdated)) {
            // Still loading.
            return html``;
        }

        let reclusteringTarget = 'updated clustering algorithms';
        let progressPerMille = progressToLatestAlgorithms(this.progress);

        const configProgress = progressToLatestConfig(this.progress);
        if (configProgress < progressPerMille) {
            reclusteringTarget = 'updated clustering configuration';
            progressPerMille = configProgress;
        }

        if (this.hasRule && this.rulePredicateLastUpdated) {
            const ruleProgress = progressToRulesVersion(this.progress, this.rulePredicateLastUpdated);
            if (ruleProgress < progressPerMille) {
                reclusteringTarget = 'the latest rule definition';
                progressPerMille = ruleProgress;
            }
        }
        this.progressPerMille = progressPerMille;

        if (progressPerMille >= 1000 && !this.show) {
            return html``;
        }

        // Once shown, keep showing.
        this.show = true;

        let progressText = 'task queued';
        if (progressPerMille >= 0) {
            progressText = (progressPerMille / 10).toFixed(1) + '%';
        }

        let content: TemplateResult;
        if (progressPerMille < 1000) {
            content = html`
            <span class="progress-description" data-cy="reclustering-progress-description">
                Weetbix is re-clustering test results to reflect ${reclusteringTarget} (${progressText}). Cluster impact may be out-of-date.
                <span class="last-updated">
                    Last update ${this.lastRefreshed?.toLocaleString(DateTime.TIME_WITH_SECONDS)}.
                </span>
            </span>`;
        } else {
            content = html`
            <span class="progress-description" data-cy="reclustering-progress-description">
                Weetbix has finished re-clustering test results. Updated cluster impact is now available.
            </span>
            <mwc-button outlined @click=${this.refreshAnalysis}>
                View Updated Impact
            </mwc-button>`;
        }

        return html`
        <div class="progress-box">
            <mwc-circular-progress
                ?indeterminate=${progressPerMille < 0}
                progress="${Math.max(0, progressPerMille / 1000)}">
            </mwc-circular-progress>
            ${content}
        </div>
        `;
    }

    async fetch() {
        this.progress = await fetchProgress(this.project);
        this.lastRefreshed = DateTime.now();
        this.requestUpdate();
    }

    refreshAnalysis() {
        this.fireRefreshAnalysis();
        this.show = false;
    }

    fireRefreshAnalysis() {
        const event = new CustomEvent<RefreshAnalysisEvent>('refreshanalysis', {
            detail: {
            },
        });
        this.dispatchEvent(event);
    }

    static styles = [css`
        .progress-box {
            display: flex;
            background-color: var(--light-active-color);
            padding: 5px;
            align-items: center;
        }
        .progress-description {
            padding: 0px 10px;
        }
        .last-updated {
            padding: 0px;
            font-size: var(--font-size-small);
            color: var(--greyed-out-text-color);
        }
    `];
}

// RefreshAnalysisEvent is an event that is triggered when the user requests
// cluster analysis to be updated.
// eslint-disable-next-line @typescript-eslint/no-empty-interface
export interface RefreshAnalysisEvent {}