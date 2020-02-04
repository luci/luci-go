import { MobxLitElement } from "@adobe/lit-mobx";
import { customElement, css, property } from "lit-element";
import { html } from "lit-html";
import * as mdcCardStyle from '@material/card/dist/mdc.card.css';
import { TestResult, TestExoneration } from '../../models/resultdb';
import '@material/mwc-checkbox';
import '@material/mwc-formfield';
import { observable, computed } from "mobx";
import { Checkbox } from "@material/mwc-checkbox";

export type TestResultFilter = (testResult: TestResult) => boolean

@customElement('tr-test-result-filters')
export class TestResultFilters extends MobxLitElement {
    public onFilterChange: (filter: TestResultFilter) => void = () => undefined;
    @observable.ref
    public testResults: TestResult[] = [];
    @observable.ref
    public testExonerations: TestExoneration[] = [];

    @computed
    private get resultStats() {
        const expected = this.testResults.filter((result) => result.expected).length || 0;
        return {
            expected,
            unexpected: (this.testResults.length || 0) - expected,
            exonerated: this.testExonerations.length,
        }
    }

    @observable.ref
    private unexpectedTests = true;
    @observable.ref
    private expectedTests = false;

    private filterResults = (testResult: TestResult) => {
        if (this.unexpectedTests && !testResult.expected) {
            return true;
        }
        if (this.expectedTests && testResult.expected) {
            return true;
        }
        return false;
    }

    protected render() {
        return html`
        <div id="filters-container">
            <mwc-formfield label=${`Expected(${this.resultStats.expected})`}>
                <mwc-checkbox
                    ?checked=${this.expectedTests}
                    @change=${(v: MouseEvent) => this.expectedTests = (v.target as Checkbox).checked}
                ></mwc-checkbox>
            </mwc-formfield>
            <mwc-formfield label=${`Unexpected(${this.resultStats.unexpected})`}>
                <mwc-checkbox
                    ?checked=${this.unexpectedTests}
                    @change=${(v: MouseEvent) => this.unexpectedTests = (v.target as Checkbox).checked}
                ></mwc-checkbox>
            </mwc-formfield>
            <span>
                <img src="/assets/svg/Filter-icon-vector-02.svg" width="25px" height="25px" class="center"/>
            </span>
        </div>
        <div id="search-box-container">
            <input id="search-box" placeholder="Search"/>
        </div>
        `;
    }

    protected firstUpdated() {
        this.onFilterChange(this.filterResults);
    }

    static styles = css`
        :host {
            display: block;
        }
        .center {
            vertical-align: middle;
        }
        #filters-container>* {
            margin: 0 5px;
        }
        #filters-container>*:first-child {
            margin-left: 0;
        }
        #filters-container>*:last-child {
            margin-right: 0;
        }
        #search-box-container {
            padding: 0 5px;
        }
        #search-box {
            width: 100%;
        }
        mwc-formfield > mwc-checkbox {
            margin-right: -5px;
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
