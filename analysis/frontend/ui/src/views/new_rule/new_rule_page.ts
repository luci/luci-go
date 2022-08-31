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
import '@material/mwc-dialog';
import '@material/mwc-icon';

import {
    css,
    customElement,
    html,
    LitElement,
    property,
    state
} from 'lit-element';
import { Ref } from 'react';
import { NavigateFunction } from 'react-router-dom';
import { URLSearchParams } from 'url';

import {
    GrpcError,
    RpcCode
} from '@chopsui/prpc-client';
import { Snackbar } from '@material/mwc-snackbar';
import { TextArea } from '@material/mwc-textarea';

import { ClusterId } from '../../services/shared_models';
import {
    CreateRuleRequest,
    getRulesService
} from '../../services/rules';
import { BugPicker } from '../../shared_elements/bug_picker';
import { linkToRule } from '../../tools/urlHandling/links';

/**
 * NewRulePage displays a page for creating a new rule in LUCI Analysis.
 * This is implemented as a page and not a pop-up dialog, as it will make it
 * easier for external integrations that want to link to the new rule
 * page in LUCI Analysis from a failure (e.g. from a failure in MILO).
 */
@customElement('new-rule-page')
export class NewRulePage extends LitElement {
    @property()
    project = '';

    @property()
    ruleString!: string | null;

    @property()
    sourceAlg!: string | null;

    @property()
    sourceId!: string | null;

    @property({ attribute: false })
    ref: Ref<NewRulePage> | null = null;

    navigate!: NavigateFunction;

    searchParams!: URLSearchParams;

    @state()
    validationMessage = '';

    @state()
    defaultRule = '';

    @state()
    sourceCluster: ClusterId = { algorithm: '', id: '' };

    @state()
    snackbarError = '';

    updateRuleAndClusterFromSearch() {
        let rule = this.ruleString;
        if (rule) {
            this.defaultRule = rule;
        }
        let sourceClusterAlg = this.sourceAlg;
        let sourceClusterID = this.sourceId;
        if (sourceClusterAlg && sourceClusterID) {
            this.sourceCluster = {
                algorithm: sourceClusterAlg,
                id: sourceClusterID,
            }
        }
    }

    connectedCallback() {
        super.connectedCallback();
        this.updateRuleAndClusterFromSearch();
        this.validationMessage = '';
        this.snackbarError = '';
    }

    render() {
        return html`
        <div id="container">
            <h1>New Rule</h1>
            <div class="validation-error" data-cy="rule-definition-validation-error">${this.validationMessage}</div>
            <div class="label">Associated Bug <mwc-icon class="inline-icon" title="The bug corresponding to the specified failures.">help_outline</mwc-icon></div>
            <bug-picker project="${this.project}" id="bug"></bug-picker>
            <div class="label">Rule Definition <mwc-icon class="inline-icon" title="A rule describing the set of failures being associated. Rules follow a subset of BigQuery Standard SQL's boolean expression syntax.">help_outline</mwc-icon></div>
            <div class="info">
                E.g. reason LIKE "%something blew up%" or test = "mytest". Supported is AND, OR, =, <>, NOT, IN, LIKE, parentheses and <a href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">REGEXP_CONTAINS</a>.
            </div>
            <mwc-textarea id="rule-definition" label="Definition" maxLength="4096" required data-cy="rule-definition-textbox" value=${this.defaultRule}></mwc-textarea>
            <mwc-button id="create-button" raised @click="${this.save}" data-cy="create-button">Create</mwc-button>
            <mwc-snackbar id="error-snackbar" labelText="${this.snackbarError}"></mwc-snackbar>
        </div>
        `;
    }

    async save() {
        const ruleDefinition = this.shadowRoot!.getElementById('rule-definition') as TextArea;
        const bugPicker = this.shadowRoot!.getElementById('bug') as BugPicker;

        this.validationMessage = '';

        const request: CreateRuleRequest = {
            parent: `projects/${this.project}`,
            rule: {
                bug: {
                    system: bugPicker.bugSystem,
                    id: bugPicker.bugId,
                },
                ruleDefinition: ruleDefinition.value,
                isActive: true,
                isManagingBug: true,
                sourceCluster: this.sourceCluster,
            },
        }

        const service = getRulesService();
        try {
            const rule = await service.create(request);
            this.validationMessage = JSON.stringify(rule);
            // Apparently .<>? doesn't work with a function
            this.navigate(linkToRule(rule.project, rule.ruleId));
            this.requestUpdate();
        } catch (e) {
            let handled = false;
            if (e instanceof GrpcError) {
                if (e.code === RpcCode.INVALID_ARGUMENT) {
                    handled = true;
                    this.validationMessage = 'Validation error: ' + e.description.trim() + '.';
                }
            }
            if (!handled) {
                this.showSnackbar(e as string);
            }
        }
    }

    showSnackbar(error: string) {
        this.snackbarError = "Creating rule: " + error;

        // Let the snackbar manage its own closure after a delay.
        const snackbar = this.shadowRoot!.getElementById("error-snackbar") as Snackbar;
        snackbar.show();
    }

    static styles = [css`
        #container {
            margin: 20px 14px;
        }
        #rule-definition {
            width: 100%;
            height: 160px;
        }
        #create-button {
            margin-top: 10px;
            float: right;
        }
        h1 {
            font-size: 18px;
            font-weight: normal;
        }
        .inline-button {
            display: inline-block;
            vertical-align: middle;
        }
        .inline-icon {
            vertical-align: middle;
            font-size: 1.5em;
        }
        .title {
            margin-bottom: 0px;
        }
        .label {
            margin-top: 15px;
        }
        .info {
            color: var(--light-text-color);
        }
        .validation-error {
            margin-top: 10px;
            color: var(--mdc-theme-error, #b00020);
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
        mwc-textarea, bug-picker {
            margin: 5px 0px;
        }
    `];
}
