/* eslint-disable import/no-duplicates */
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

import { LitElement, html, customElement, property, css, state, TemplateResult } from 'lit-element';
import { DateTime } from 'luxon';
import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import '@material/mwc-button';
import '@material/mwc-dialog';
import '@material/mwc-textfield';
import '@material/mwc-textarea';
import { TextArea } from '@material/mwc-textarea';
import '@material/mwc-textfield';
import '@material/mwc-snackbar';
import { Snackbar } from '@material/mwc-snackbar';
import '@material/mwc-switch';
import { Switch } from '@material/mwc-switch';
import '@material/mwc-icon';
import { BugPicker } from '../../../../shared_elements/bug_picker';
import '../../../../shared_elements/bug_picker';

import { getRulesService, Rule, UpdateRuleRequest } from '../../../../services/rules';
import { getIssuesService, Issue, GetIssueRequest } from '../../../../services/monorail';
import { linkToCluster } from '../../../../tools/urlHandling/links';

/**
 * RuleSection displays a rule tracked by LUCI Analysis.
 * @fires rulechanged
 */
@customElement('rule-section')
export class RuleSection extends LitElement {
    @property()
    project = '';

    @property()
    ruleId = '';

    @state()
    rule: Rule | null = null;

    @state()
    issue: Issue | null = null;

    @state()
    editingRule = false;

    @state()
    editingBug = false;

    @state()
    validationMessage = '';

    @state()
    snackbarError = '';

    connectedCallback() {
        super.connectedCallback();
        this.fetch();
    }

    render() {
        if (!this.rule) {
            return html`Loading...`;
        }
        const r = this.rule;
        const formatTime = (time: string): string => {
            const t = DateTime.fromISO(time);
            const d = DateTime.now().diff(t);
            if (d.as('seconds') < 60) {
                return 'just now';
            }
            if (d.as('hours') < 24) {
                return t.toRelative()?.toLocaleLowerCase() || '';
            }
            return DateTime.fromISO(time).toLocaleString(DateTime.DATETIME_SHORT);
        };
        const formatTooltipTime = (time: string): string => {
            // Format date/time with full month name, e.g. "January" and Timezone,
            // to disambiguate date/time even if the user's locale has been set
            // incorrectly.
            return DateTime.fromISO(time).toLocaleString(DateTime.DATETIME_FULL_WITH_SECONDS);
        };
        const bugStatusClass = (status: string): string => {
            // In monorail, bug statuses are configurable per system. Right now,
            // we don't have a configurable mapping from status to semantic in
            // LUCI Analysis. We will try to recognise common terminology and
            // fall back to "other" status otherwise.
            status = status.toLowerCase();
            const unassigned = ['new', 'untriaged', 'available'];
            const assigned = ['accepted', 'assigned', 'started', 'externaldependency'];
            const fixed = ['fixed', 'verified'];
            if (unassigned.indexOf(status) >= 0) {
                return 'bug-status-unassigned';
            } else if (assigned.indexOf(status) >= 0) {
                return 'bug-status-assigned';
            } else if (fixed.indexOf(status) >= 0) {
                return 'bug-status-fixed';
            } else {
                // E.g. Won't fix, duplicate, archived.
                return 'bug-status-other';
            }
        };
        const formatUser = (user: string): TemplateResult => {
            if (user == 'system') {
                return html`LUCI Analysis`;
            } else if (user.endsWith('@google.com')) {
                const ldap = user.substr(0, user.length - '@google.com'.length);
                return html`<a href="http://who/${ldap}">${ldap}</a>`;
            } else {
                return html`${user}`;
            }
        };
        return html`
        <div>
            <h1 data-cy="rule-title">${this.issue != null ? this.issue.summary : '...' }</h1>
            <div class="definition-box-container">
                <pre class="definition-box" data-cy="rule-definition">${r.ruleDefinition}</pre>
                <div class="definition-edit-button">
                    <mwc-button outlined @click="${this.editRule}" data-cy="rule-definition-edit">Edit</mwc-button>
                </div>
            </div>
            <table>
                <tbody>
                    <tr>
                        <th>Associated Bug</th>
                        <td data-cy="bug">
                            <a href="${r.bug.url}">${r.bug.linkText}</a>
                            <div class="inline-button">
                                <mwc-button outlined dense @click="${this.editBug}" data-cy="bug-edit">Edit</mwc-button>
                            </div>
                        </td>
                    </tr>
                    <tr>
                        <th>Status</th>
                        <td data-cy="bug-status">${this.issue != null ? html`<span class="bug-status ${bugStatusClass(this.issue.status.status)}">${this.issue.status.status}</span>` : html`...` }</td>
                    </tr>
                    <tr>
                        <th>Bug Updates <mwc-icon class="inline-icon" title="Whether the priority and verified status of the associated bug should be automatically updated based on cluster impact. Only one rule may be set to update a given bug at any one time.">help_outline</mwc-icon></th>
                        <td data-cy="bug-updates">
                            <mwc-switch id="bug-updates-toggle" data-cy="bug-updates-toggle" .selected=${r.isManagingBug} @click=${this.toggleManagingBug}></mwc-switch>
                        </td>
                    </tr>
                    <tr>
                        <th>Archived <mwc-icon class="inline-icon" title="Archived failure association rules do not match failures. If a rule is no longer needed, it should be archived.">help_outline</mwc-icon></th>
                        <td data-cy="rule-archived">
                            ${r.isActive ? 'No' : 'Yes'}
                            <div class="inline-button">
                                <mwc-button outlined dense @click="${this.toggleArchived}" data-cy="rule-archived-toggle">${r.isActive ? 'Archive' : 'Restore'}</mwc-button>
                            </div>
                        </td>
                    </tr>
                    <tr>
                        <th>Source Cluster <mwc-icon class="inline-icon" title="The cluster this rule was originally created from.">help_outline</mwc-icon></th>
                        <td>
                            ${r.sourceCluster.algorithm && r.sourceCluster.id ?
                                html`<a href="${linkToCluster(this.project, r.sourceCluster)}">${r.sourceCluster.algorithm}/${r.sourceCluster.id}</a>` :
                                html`None`
                            }
                        </td>
                    </tr>
                </tbody>
            </table>
            <div class="audit">
                ${(r.lastUpdateTime != r.createTime) ?
                html`Last updated by <span class="user">${formatUser(r.lastUpdateUser)}</span> <span class="time" title="${formatTooltipTime(r.lastUpdateTime)}">${formatTime(r.lastUpdateTime)}</span>.` : html``}
                Created by <span class="user">${formatUser(r.createUser)}</span> <span class="time" title="${formatTooltipTime(r.createTime)}">${formatTime(r.createTime)}</span>.
            </div>
        </div>
        <mwc-dialog class="rule-edit-dialog" .open="${this.editingRule}" @closed="${this.editRuleClosed}">
            <div class="edit-title">Edit Rule Definition <mwc-icon class="inline-icon" title="LUCI Analysis rule definitions describe the failures associated with a bug. Rules follow a subset of BigQuery Standard SQL's boolean expression syntax.">help_outline</mwc-icon></div>
            <div class="validation-error" data-cy="rule-definition-validation-error">${this.validationMessage}</div>
            <mwc-textarea id="rule-definition" label="Rule Definition" maxLength="4096" required data-cy="rule-definition-textbox"></mwc-textarea>
            <div>
                Supported is AND, OR, =, <>, NOT, IN, LIKE, parentheses and <a href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">REGEXP_CONTAINS</a>.
                Valid identifiers are <em>test</em> and <em>reason</em>.
            </div>
            <mwc-button slot="primaryAction" @click="${this.saveRule}" data-cy="rule-definition-save">Save</mwc-button>
            <mwc-button slot="secondaryAction" dialogAction="close" data-cy="rule-definition-cancel">Cancel</mwc-button>
        </mwc-dialog>
        <mwc-dialog class="bug-edit-dialog" .open="${this.editingBug}" @closed="${this.editBugClosed}">
            <div class="edit-title">Edit Associated Bug</div>
            <div class="validation-error" data-cy="bug-validation-error">${this.validationMessage}</div>
            <bug-picker id="bug" project="${this.project}" material832Workaround></bug-picker>
            <mwc-button slot="primaryAction" @click="${this.saveBug}" data-cy="bug-save">Save</mwc-button>
            <mwc-button slot="secondaryAction" dialogAction="close" data-cy="bug-cancel">Cancel</mwc-button>
        </mwc-dialog>
        <mwc-snackbar id="error-snackbar" labelText="${this.snackbarError}"></mwc-snackbar>
        `;
    }

    async fetch() {
        if (!this.ruleId) {
            throw new Error('rule-section element ruleID property is required');
        }
        const service = getRulesService();
        const rule = await service.get({
            name: `projects/${this.project}/rules/${this.ruleId}`
        });

        this.rule = rule;
        this.fireRuleChanged();
        this.fetchBug(rule);
        this.requestUpdate();
    }

    async fetchBug(rule: Rule) {
        if (rule.bug.system === 'monorail') {
            const parts = rule.bug.id.split('/');
            const monorailProject = parts[0];
            const bugId = parts[1];
            const issueId = `projects/${monorailProject}/issues/${bugId}`;
            if (this.issue != null && this.issue.name == issueId) {
                // No update required.
                return;
            }
            this.issue = null;
            const service = getIssuesService();
            const request: GetIssueRequest = {
                name: issueId
            };
            const issue = await service.getIssue(request);
            this.issue = issue;
        } else {
            this.issue = null;
        }
        this.requestUpdate();
    }

    editRule() {
        if (!this.rule) {
            throw new Error('invariant violated: editRule cannot be called before rule is loaded');
        }
        const ruleDefinition = this.shadowRoot!.getElementById('rule-definition') as TextArea;
        ruleDefinition.value = this.rule.ruleDefinition;

        this.editingRule = true;
        this.validationMessage = '';
    }

    editBug() {
        if (!this.rule) {
            throw new Error('invariant violated: editBug cannot be called before rule is loaded');
        }
        const picker = this.shadowRoot!.getElementById('bug') as BugPicker;
        picker.bugSystem = this.rule.bug.system;
        picker.bugId = this.rule.bug.id;

        this.editingBug = true;
        this.validationMessage = '';
    }

    editRuleClosed() {
        this.editingRule = false;
    }

    editBugClosed() {
        this.editingBug = false;
    }

    async saveRule() {
        if (!this.rule) {
            throw new Error('invariant violated: saveRule cannot be called before rule is loaded');
        }
        const ruleDefinition = this.shadowRoot!.getElementById('rule-definition') as TextArea;
        if (ruleDefinition.value == this.rule.ruleDefinition) {
            this.editingRule = false;
            return;
        }

        this.validationMessage = '';

        const request: UpdateRuleRequest = {
            rule: {
                name: this.rule.name,
                ruleDefinition: ruleDefinition.value,
            },
            updateMask: 'ruleDefinition',
            etag: this.rule.etag,
        };

        try {
            await this.applyUpdate(request);
            this.editingRule = false;
        } catch (e) {
            this.routeUpdateError(e);
        }
        this.requestUpdate();
    }

    async saveBug() {
        if (!this.rule) {
            throw new Error('invariant violated: saveBug cannot be called before rule is loaded');
        }
        const picker = this.shadowRoot!.getElementById('bug') as BugPicker;
        if (picker.bugSystem === this.rule.bug.system && picker.bugId === this.rule.bug.id) {
            this.editingBug = false;
            return;
        }

        this.validationMessage = '';

        const request: UpdateRuleRequest = {
            rule: {
                name: this.rule.name,
                bug: {
                    system: picker.bugSystem,
                    id: picker.bugId,
                },
            },
            updateMask: 'bug',
            etag: this.rule.etag,
        };

        try {
            await this.applyUpdate(request);
            this.editingBug = false;
            this.requestUpdate();
        } catch (e) {
            this.routeUpdateError(e);
        }
    }

    // routeUpdateError is used to handle update errors that occur in the
    // context of a model dialog, where a validation err message can be
    // displayed.
    routeUpdateError(e: any) {
        if (e instanceof GrpcError) {
            if (e.code === RpcCode.INVALID_ARGUMENT) {
                this.validationMessage = 'Validation error: ' + e.description.trim() + '.';
                return;
            }
        }
        this.showSnackbar(e as string);
    }

    async toggleArchived() {
        if (!this.rule) {
            throw new Error('invariant violated: toggleActive cannot be called before rule is loaded');
        }
        const request: UpdateRuleRequest = {
            rule: {
                name: this.rule.name,
                isActive: !this.rule.isActive,
            },
            updateMask: 'isActive',
            etag: this.rule.etag,
        };
        try {
            await this.applyUpdate(request);
            this.requestUpdate();
        } catch (err) {
            this.showSnackbar(err as string);
        }
    }

    async toggleManagingBug() {
        if (!this.rule) {
            throw new Error('invariant violated: toggleManagingBug cannot be called before rule is loaded');
        }

        this.requestUpdate();
        // Revert the automatic toggle caused by the click.
        const toggle = this.shadowRoot!.getElementById('bug-updates-toggle') as Switch;
        toggle.selected = this.rule.isManagingBug;

        const request: UpdateRuleRequest = {
            rule: {
                name: this.rule.name,
                isManagingBug: !this.rule.isManagingBug,
            },
            updateMask: 'isManagingBug',
            etag: this.rule.etag,
        };
        try {
            await this.applyUpdate(request);
            this.requestUpdate();
        } catch (err) {
            this.showSnackbar(err as string);
        }
    }

    // applyUpdate tries to apply the given update to the rule. If the
    // update succeeds, this method returns nil. If a validation error
    // occurs, the validation message is returned.
    async applyUpdate(request: UpdateRuleRequest) : Promise<void> {
        const service = getRulesService();
        const rule = await service.update(request);
        this.rule = rule;
        this.fireRuleChanged();
        this.fetchBug(rule);
        this.requestUpdate();
    }

    showSnackbar(error: string) {
        this.snackbarError = 'Updating rule: ' + error;

        // Let the snackbar manage its own closure after a delay.
        const snackbar = this.shadowRoot!.getElementById('error-snackbar') as Snackbar;
        snackbar.show();
    }

    fireRuleChanged() {
        if (!this.rule) {
            throw new Error('invariant violated: fireRuleChanged cannot be called before rule is loaded');
        }
        const event = new CustomEvent<RuleChangedEvent>('rulechanged', {
            detail: {
                predicateLastUpdated: this.rule.predicateLastUpdateTime,
            },
        });
        this.dispatchEvent(event);
    }

    static styles = [css`
        h1 {
            font-size: 18px;
            font-weight: normal;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }
        span.bug-status {
            color: white;
            border-radius: 20px;
            padding: 2px 8px;
        }
        /* Open an unassigned. E.g. New, Untriaged, Available bugs. */
        span.bug-status-unassigned {
            background-color: #b71c1c;
        }
        /* Open and assigned. E.g. Assigned, Started. */
        span.bug-status-assigned {
            background-color: #0d47a1;
        }
        /* Closed and fixed. E.g. Fixed, Verified. */
        span.bug-status-fixed {
            background-color: #2e7d32;
        }
        /* Closed and not fixed and other statuses we don't recognise.
           E.g. Won't fix, Duplicate, Archived */
        span.bug-status-other {
            background-color: #000000;
        }
        .inline-button {
            display: inline-block;
            vertical-align: middle;
            padding-left: 5px;
        }
        .inline-icon {
            vertical-align: middle;
            font-size: 1.5em;
        }
        .edit-title {
            margin-bottom: 10px;
        }
        .rule-edit-dialog {
            --mdc-dialog-min-width:1000px
        }
        .validation-error {
            color: var(--mdc-theme-error, #b00020);
        }
        #rule-definition {
            width: 100%;
            height: 160px;
        }
        .definition-box-container {
            display: flex;
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
        .definition-edit-button {
            align-self: center;
            margin: 5px;
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
            height: 28px;
        }
        mwc-textarea, bug-picker {
            margin: 5px 0px;
        }
        .audit {
            font-size: var(--font-size-small);
            color: var(--greyed-out-text-color);
        }
    `];
}

export interface RuleChangedEvent {
    predicateLastUpdated: string; // RFC 3339 encoded date/time.
}
