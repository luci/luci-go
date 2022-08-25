
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

import './elements/reclustering_progress_indicator';
import '../../../shared_elements/failure_table';
import './elements/rule_section';
import './elements/impact_table';

import {
    css,
    customElement,
    html,
    LitElement,
    property,
    state,
    TemplateResult,
} from 'lit-element';
import { Ref } from 'react';
import { NavigateFunction } from 'react-router-dom';

import { Cluster, BatchGetClustersRequest, getClustersService } from '../../../services/cluster';
import { RuleChangedEvent } from './elements/rule_section';

// ClusterPage lists the clusters tracked by Weetbix.
@customElement('cluster-page')
export class ClusterPage extends LitElement {

    @property({ attribute: false })
    ref: Ref<ClusterPage> | null = null;

    @property()
    project = '';

    @property()
    clusterAlgorithm = '';

    @property()
    clusterId = '';

    navigate!: NavigateFunction;

    @state()
    cluster: Cluster | undefined;

    @state()
    // When the displayed rule's predicate (if any) was last updated.
    // This is provided to the reclustering progress indicator to show
    // the correct re-clustering status.
    rulePredicateLastUpdated = '';

    connectedCallback() {
        super.connectedCallback();
        this.rulePredicateLastUpdated = '';
        this.refreshAnalysis();
    }

    render() {
        const currentCluster = this.cluster;

        let definitionSection = html`Loading...`;
        if (this.clusterAlgorithm.startsWith('rules-')) {
            definitionSection = html`
                <rule-section project=${this.project} ruleId=${this.clusterId} @rulechanged=${this.onRuleChanged}>
                </rule-section>
            `;
        } else if (currentCluster !== undefined) {
            let criteriaName = '';
            if (this.clusterAlgorithm.startsWith('testname-')) {
                criteriaName = 'Test name-based clustering';
            } else if (this.clusterAlgorithm.startsWith('reason-')) {
                criteriaName = 'Failure reason-based clustering';
            }
            let newRuleButton: TemplateResult = html``;
            if (currentCluster.equivalentFailureAssociationRule) {
                newRuleButton = html`<mwc-button class="new-rule-button" raised @click=${this.newRuleClicked}>New Rule from Cluster</mwc-button>`;
            }

            definitionSection = html`
            <h1>Cluster <span class="cluster-id">${this.clusterAlgorithm}/${this.clusterId}</span></h1>
            <div class="definition-box-container">
                <pre class="definition-box">${currentCluster.hasExample ? currentCluster.title : '(cluster no longer exists)'}</pre>
            </div>
            <table class="definition-table">
                <tbody>
                    <tr>
                        <th>Type</th>
                        <td>Suggested</td>
                    </tr>
                    <tr>
                        <th>Algorithm</th>
                        <td>${criteriaName}</td>
                    </tr>
                </tbody>
            </table>
            ${newRuleButton}
            `;
        }

        let impactTable = html`Loading...`;
        if (currentCluster !== undefined) {
            impactTable = html`
            <impact-table .currentCluster=${currentCluster}></impact-table>
            `;
        }

        return html`
        <reclustering-progress-indicator project=${this.project} ?hasrule=${this.clusterAlgorithm.startsWith('rules-')}
            rulePredicateLastUpdated=${this.rulePredicateLastUpdated} @refreshanalysis=${this.refreshAnalysis}>
        </reclustering-progress-indicator>
        <div id="container">
            ${definitionSection}
            <h2>Impact</h2>
            ${impactTable}
            <h2>Recent Failures</h2>
            <failure-table project=${this.project} clusterAlgorithm=${this.clusterAlgorithm} clusterID=${this.clusterId}>
            </failure-table>
        </div>
        `;
    }

    newRuleClicked() {
        if (!this.cluster) {
            throw new Error('invariant violated: newRuleClicked cannot be called before cluster is loaded');
        }
        const projectEncoded = encodeURIComponent(this.project);
        const ruleEncoded = encodeURIComponent(this.cluster.equivalentFailureAssociationRule || '');
        const sourceAlgEncoded = encodeURIComponent(this.clusterAlgorithm);
        const sourceIdEncoded = encodeURIComponent(this.clusterId);

        const newRuleURL = `/p/${projectEncoded}/rules/new?rule=${ruleEncoded}&sourceAlg=${sourceAlgEncoded}&sourceId=${sourceIdEncoded}`;
        this.navigate(newRuleURL);
    }

    // Called when the rule displayed in the rule section is loaded
    // for the first time, or updated.
    onRuleChanged(e: CustomEvent<RuleChangedEvent>) {
        this.rulePredicateLastUpdated = e.detail.predicateLastUpdated;
    }

    // (Re-)loads cluster impact analysis. Called on page load or
    // if the refresh button on the reclustering progress indicator
    // is clicked at completion of re-clustering.
    async refreshAnalysis() {
        this.cluster = undefined;
        const service = getClustersService();
        const request: BatchGetClustersRequest = {
            parent: `projects/${encodeURIComponent(this.project)}`,
            names: [
                `projects/${encodeURIComponent(this.project)}/clusters/${encodeURIComponent(this.clusterAlgorithm)}/${encodeURIComponent(this.clusterId)}`,
            ],
        };

        const response = await service.batchGet(request);

        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        this.cluster = response.clusters![0];
        this.requestUpdate();
    }

    static styles = [css`
        #container {
            margin: 20px 14px;
        }
        h1 {
            font-size: 18px;
            font-weight: normal;
        }
        h2 {
            margin-top: 40px;
            font-size: 16px;
            font-weight: normal;
        }
        .cluster-id {
            font-family: monospace;
            font-size: 80%;
            background-color: var(--light-active-color);
            border: solid 1px var(--active-color);
            border-radius: 20px;
            padding: 2px 8px;
        }
        .definition-box-container {
            margin-bottom: 20px;
        }
        .definition-box {
            border: solid 1px var(--divider-color);
            background-color: var(--block-background-color);
            padding: 20px 14px;
            margin: 0px;
            display: inline-block;
            white-space: pre-wrap;
            overflow-wrap: anywhere;
        }
        .new-rule-button {
            margin-top: 10px;
        }
        table {
            border-collapse: collapse;
            max-width: 100%;
        }
        th {
            font-weight: normal;
            color: var(--greyed-out-text-color);
            text-align: left;
        }
        td,th {
            padding: 4px;
            max-width: 80%;
        }
        td.number {
            text-align: right;
        }
        tbody.data tr:hover {
            background-color: var(--light-active-color);
        }
    `];
}
