import { MobxLitElement } from "@adobe/lit-mobx";
import { customElement, css, property } from "lit-element";
import { html } from "lit-html";
import * as mdcCardStyle from '@material/card/dist/mdc.card.css';
import { TestResult } from "../../models/build";
import '@material/mwc-checkbox';
import '@material/mwc-formfield';
import { observable } from "mobx";
import { Checkbox } from "@material/mwc-checkbox";

export type TestResultFilter = (testResult: TestResult) => boolean

@customElement('milo-test-result-filters')
export class TestResultFilters extends MobxLitElement {
    public onFilterChange: (filter: TestResultFilter) => void = () => undefined;

    @observable
    private includeExpected = false;
    @observable
    private includeFlakyTests = true;

    private filterResults = (testResult: TestResult) => {
        if (!this.includeExpected && testResult.expected) {
            return false;
        }
        return true;
    }

    protected render() {
        return html`
        <div>
            <img src="/assets/svg/Filter-icon-vector-02.svg" width="25px" height="25px" class="center"/>
            <span>Applied</span>
        </div>
        <mwc-formfield label="Expected">
            <mwc-checkbox
                ?checked=${this.includeExpected}
                @change=${(v: MouseEvent) => this.includeExpected = (v.target as Checkbox).checked}
            ></mwc-checkbox>
        </mwc-formfield>
        <mwc-formfield label="Flaky Tests">
            <mwc-checkbox
                ?checked=${this.includeFlakyTests}
                @change=${(v: MouseEvent) => this.includeFlakyTests = (v.target as Checkbox).checked}
            ></mwc-checkbox>
        </mwc-formfield>
        <input id="search-box" placeholder="Search"/>
        `;
    }

    protected firstUpdated() {
        this.onFilterChange(this.filterResults);
    }

    static styles = css`
        :host {
            display: flex;
            align-items: baseline;
        }
        :host>* {
            margin: 0 5px 0 5px;
        }
        :host>*:first-child {
            margin-left: 0;
        }
        :host>*:last-child {
            margin-right: 0;
        }
        .center {
            vertical-align: middle;
        }
        #search-box {
            flex-grow: 1;
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
