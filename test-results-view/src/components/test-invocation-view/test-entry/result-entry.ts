import { customElement, property, html, css } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { TestResult, TestStatus } from '../../../models/resultdb';
import { classMap } from "lit-html/directives/class-map";
import { unsafeHTML } from "lit-html/directives/unsafe-html";
import "@material/mwc-icon";

import './artifact-view';
import { observable, computed } from "mobx";
import { repeat } from "lit-html/directives/repeat";
import { styleMap } from "lit-html/directives/style-map";

const STATUS_CLASS_MAP = {
    [TestStatus.Unspecified]: 'unspecified',
    [TestStatus.Pass]: 'pass',
    [TestStatus.Fail]: 'fail',
    [TestStatus.Skip]: 'skip',
    [TestStatus.Crash]: 'crash',
    [TestStatus.Abort]: 'abort',
};

const STATUS_DISPLAY_MAP = {
    [TestStatus.Unspecified]: 'unspecified',
    [TestStatus.Pass]: 'passed',
    [TestStatus.Fail]: 'failed',
    [TestStatus.Skip]: 'skipped',
    [TestStatus.Crash]: 'crashed',
    [TestStatus.Abort]: 'aborted',
};

@customElement('tr-result-entry')
export class ResultEntry extends MobxLitElement {
    @observable.ref
    public testResult?: TestResult;

    @observable.ref
    private expanded: boolean = true;
    @observable.ref
    private outputArtifactsExpanded: boolean = true;
    @observable.ref
    private inputArtifactsExpanded: boolean = false;
    @observable.ref
    private summaryExpanded: boolean = true;
    @observable.ref
    private variantExpanded: boolean = false;
    @observable.ref
    private tagExpanded: boolean = false;

    private renderVariant() {
        const variantKV = Object.entries(this.testResult!.variant?.def ?? {});
        if (variantKV.length === 0) {
            return html``;
        }
        return html`
        <div
            class="expandable-header"
            @click=${() => this.variantExpanded = !this.variantExpanded}
        >
            <mwc-icon class="expand-toggle">${this.variantExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
            <span class="one-line-content">
                Variant:
                <span class=${classMap({hidden: this.variantExpanded, light: true})}>
                ${variantKV.map(([k, v]) => html`
                    <span class="kv-key">${k}</span>:
                    <span class="kv-value">${v}</span>
                `)}
                </span>
            </span>
        </div>
        <ul class=${classMap({hidden: !this.variantExpanded})}>
            ${variantKV.map(([k, v]) => html`<li>${k}: ${v}</li>`)}
        </ul>
        `;
    }

    private renderTags() {
        return html`
        <div class=${classMap({hidden: !this.testResult!.tags.length})}>
            <div
                class="expandable-header"
                @click=${() => this.tagExpanded = !this.tagExpanded}>
                <mwc-icon class="expand-toggle">${this.tagExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                <span class="one-line-content">
                    Tags:
                    <span class=${classMap({hidden: this.tagExpanded, light: true})}>
                    ${this.testResult!.tags.map((tag) => html`
                        <span class="kv-key">${tag.key}</span>:
                        <span class="kv-value">${tag.value}</span>
                    `)}
                    </span>
                </span>
            </div>
            <ul class=${classMap({hidden: !this.tagExpanded})}>
                ${this.testResult!.tags.map((tag) => html`<li>${tag.key}: ${tag.value}</li>`)}
            </ul>
        </div>
        `
    }

    protected render() {
        return html`
        <div>
            <div
                class="expandable-header"
                @click=${() => this.expanded = !this.expanded}
            >
                <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                <span class="one-line-content">
                    #${this.testResult!.resultId}
                    ${this.testResult!.expected ? html`<span style="color: rgb(51, 172, 113);">expectedly</span>` : html`<span style="color: rgb(210, 63, 49);">unexpectedly</span>`}
                    <span id="status" class=${STATUS_CLASS_MAP[this.testResult!.status]}>
                        ${STATUS_DISPLAY_MAP[this.testResult!.status]}
                    </span> after ${this.testResult!.duration || '-s'}
                </span>
            </div>
            <div id="body">
                <div id="content-ruler"></div>
                <div id="content" class=${classMap({hidden: !this.expanded})}>
                    ${this.renderVariant()}
                    <div class=${classMap({hidden: (this.testResult!.summaryHtml ?? '').length === 0})}>
                        <div
                            class="expandable-header"
                            @click=${() => this.summaryExpanded = !this.summaryExpanded}
                        >
                            <mwc-icon class="expand-toggle">${this.summaryExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                            <div class="one-line-content">Summary:</div>
                        </div>
                        <div id="summary-html" class=${classMap({hidden: !this.summaryExpanded})}>
                            ${unsafeHTML(this.testResult!.summaryHtml)}
                        </div>
                    </div>
                    ${this.renderTags()}
                    <div class=${classMap({hidden: !this.testResult!.outputArtifacts})}>
                        <div
                            class="expandable-header"
                            @click=${() => this.outputArtifactsExpanded = !this.outputArtifactsExpanded}
                        >
                            <mwc-icon class="expand-toggle">${this.outputArtifactsExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                            <div class="one-line-content">
                                Output Artifacts:
                                <span class="light">${this.testResult!.outputArtifacts?.length || 0} artifact(s)</span>
                            </div>
                        </div>
                        <ul class=${classMap({hidden: !this.outputArtifactsExpanded})}>
                            ${this.testResult!.outputArtifacts?.map((artifact) => html`
                            <li><tr-artifact-view .artifact=${artifact}></tr-artifact-view></li>
                            `)}
                        </ul>
                    </div>
                    <div class=${classMap({hidden: !this.testResult!.inputArtifacts})}>
                        <div
                            class="expandable-header"
                            @click=${() => this.inputArtifactsExpanded = !this.inputArtifactsExpanded}
                        >
                            <mwc-icon class="expand-toggle">${this.inputArtifactsExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                            <div class="one-line-content">
                                Input Artifacts:
                                <span class="light">${this.testResult!.inputArtifacts?.length || 0} artifact(s)</span>
                            </div>
                        </div>
                        <ul class=${classMap({hidden: !this.inputArtifactsExpanded})}>
                            ${this.testResult!.inputArtifacts?.map((artifact) => html`
                            <li><tr-artifact-view .artifact=${artifact}></tr-artifact-view></li>
                            `)}
                        </ul>
                    </div>
                </div>
            </div>
        </div>
        `;
    }

    static styles = css`
    .hidden {
        display: none;
    }

    .expandable-header {
        display: grid;
        grid-template-columns: 24px 1fr;
        grid-template-rows: 24px;
        grid-gap: 5px;
        cursor: pointer;
    }
    .expandable-header .expand-toggle {
        grid-row: 1;
        grid-column: 1;
    }
    .expandable-header .one-line-content {
        grid-row: 1;
        grid-column: 2;
        font-size: 16px;
        line-height: 24px;
        overflow: hidden;
        white-space: nowrap;
        text-overflow: ellipsis;
    }

    #status.pass {
        color: rgb(51, 172, 113);
    }
    #status.fail {
        color: rgb(210, 63, 49);
    }
    #status.crash {
        color: rgb(51, 172, 113);
    }
    #status.abort {
        color: rgb(51, 172, 113);
    }
    #status.skip {
        color: grey;
    }
    #status.unspecified {
        color: grey;
    }

    #body {
        display: grid;
        grid-template-columns: 24px 1fr;
        grid-gap: 5px;
    }
    #content-ruler {
        border-left: 1px solid #DDDDDD;
        width: 0px;
        margin-left: 11.5px;
    }
    #summary-html {
        background-color: rgb(245, 245, 245);
        padding: 5px;
    }

    .light {
        color: grey;
    }
    .kv-value::after {
        content: ',';
    }
    .kv-value:last-child::after {
        content: '';
    }
    `;
}
