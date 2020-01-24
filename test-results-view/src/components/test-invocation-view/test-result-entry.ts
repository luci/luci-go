import { customElement, property, html, css } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { TestResult } from "../../models/build";
import { classMap } from "lit-html/directives/class-map";
import * as mdcCardStyle from '@material/card/dist/mdc.card.css';
import "@material/mwc-icon";

import './artifact-view';
import moment from "moment";
import { observable } from "mobx";

@customElement('milo-test-result-entry')
export class TestResultEntry extends MobxLitElement {
    @observable.ref
    public testResult?: TestResult;
    @observable.ref
    public expanded: boolean = false;
    public onHeaderClicked: () => void = () => {};

    protected render() {
        return html`
        <div class=${classMap({
            expected: this.testResult!.expected || false,
            expanded: this.expanded,
        })}>
            <div
                id="header"
                @click=${() => this.onHeaderClicked()}
            >
                <mwc-icon id="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
                <mwc-icon id="expectancy-indicator">${this.testResult!.expected ? 'check_circle' : 'error'}</mwc-icon>
                <div id="test-identifier" title="Test ID -> Result ID">
                    ${this.testResult!.testId} -> <strong>${this.testResult!.resultId}</strong>
                </div>
                <div id="test-variant">${Object.entries(this.testResult!.variant?.def || {})
                    .sort(([key1], [key2]) => key1 === key2 ? 0 : key1 < key2 ? -1 : 1)
                    .map(([k, v]) => `${k}: ${v}`).join(', ')
                }</div>
            </div>
            <div id="content">
                <div id="content-header">
                    <div id="status">Status: ${this.testResult!.status}</div>
                    <div id="start-time">Start Time: ${this.testResult!.startTime ? moment(this.testResult!.startTime).calendar() : '-'}</div>
                    <div id="duration">Duration: ${this.testResult!.duration ? this.testResult!.duration : '-'}</div>
                </div>
                <div class=${classMap({hidden: !this.testResult!.summaryHtml})}>Summary:<br/> ${this.testResult!.summaryHtml}</div>
                <div class=${classMap({hidden: !this.testResult!.tags.length})}>Tags:
                    <ul>${this.testResult!.tags.map((tag) => html`<li>${tag.key}: ${tag.value}</li>`)}</ul>
                </div>
                <div class=${classMap({hidden: !this.testResult!.outputArtifacts})}>Output Artifacts:
                    <ul>
                        ${this.testResult!.outputArtifacts?.map((artifact) => html`
                        <li><milo-artifact-view .artifact=${artifact}></milo-artifact-view></li>
                        `)}
                    </ul>
                </div>
                <div class=${classMap({hidden: !this.testResult!.inputArtifacts})}>Input Artifacts:
                    <ul>
                        ${this.testResult!.inputArtifacts?.map((artifact) => html`
                        <li><milo-artifact-view .artifact=${artifact}></milo-artifact-view></li>
                        `)}
                    </ul>
                </div>
            </div>
        </div>
        `;
    }

    static styles = css`
    .mdc-card {
        padding: 5px;
    }
    #content {
        padding: 5px;
        margin-left: 34px;
        display: none;
    }
    .expanded #content {
        display: block;
    }

    #content-header {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        grid-gap: 5px;
    }

    #header {
        display: grid;
        grid-template-columns: 24px 10px 24px 10px 1fr;
        grid-template-rows: 16px 8px 8px;
        cursor: pointer;
    }

    #header #expand-toggle {
        grid-row: 1/3;
        grid-column: 1;
    }

    #header #expectancy-indicator {
        color: #d23f31;
        grid-row: 1/3;
        grid-column: 3;
    }
    .expected #header #expectancy-indicator {
        color: #33ac71;
    }

    #test-identifier {
        color: #d23f31;
        grid-row: 1;
        grid-column: 5;
        font-size: 16px;
    }
    .expected #test-identifier {
        color: #33ac71;
    }

    #test-variant {
        font-size: 12px;
        grid-row: 2/4;
        grid-column: 5;
    }

    .hidden {
        display: none;
    }
    `;

    constructor() {
        super();
        (this.shadowRoot as any).adoptedStyleSheets = [
            mdcCardStyle,
            ...(this.shadowRoot as any).adoptedStyleSheets,
        ];
    }
}
