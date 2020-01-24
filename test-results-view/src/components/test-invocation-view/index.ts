import { MobxLitElement } from '@adobe/lit-mobx';
import { html, css, customElement } from 'lit-element';
import { computed, observable, IObservableValue, action } from 'mobx';
import { repeat } from 'lit-html/directives/repeat';
import { classMap } from 'lit-html/directives/class-map';
import * as mdcCardStyle from '@material/card/dist/mdc.card.css';
import '@material/mwc-icon';
import '@material/mwc-icon-button';

import { TestResult, Invocation, TestExoneration } from '../../models/build';
import { TestResultFilter } from './test-result-filters';
import { store } from '../../store';
import './test-invocation-additional-info';
import './test-result-filters';
import './test-result-entry';
import '../../lib/components/paginator';

@customElement('milo-test-invocation-view')
export class TestInvocationView extends MobxLitElement {
    @observable.ref
    private invocationName: string = "";

    @observable.ref
    private timestamp: number = Date.now();

    @observable.ref
    private filter: TestResultFilter = () => true;

    @observable
    private expandedTestResults = new Map<string, boolean>();

    @observable.ref
    private pageSize = 10;
    @observable.ref
    private startIndex = 0;

    @computed
    private get invocation() {
        const ret = observable.box(null as Invocation | null);
        if (!store.isSignedIn) {
            return ret;
        }
        const authRes = store.googleAuth!.currentUser.get().getAuthResponse();
        // artificially depends on the timestamp
        // so the API call can be treated as a pure function of f(timestamp, request)
        this.timestamp;
        fetch(
            'https://staging.results.api.cr.dev/prpc/luci.resultdb.rpc.v1.ResultDB/GetInvocation',
            {
                method: 'POST',
                headers: {
                    accept: 'application/json',
                    'content-type': 'application/json',
                    authorization: `${(authRes as any).token_type} ${authRes.access_token}`,
                },
                body: JSON.stringify({
                    "name": this.invocationName
                }),
            },
        ).then(res => res.text())
            .then(res => JSON.parse(res.slice(4)) as Invocation)
            .then(res => ret.set(res));

        return ret;
    }

    @computed
    private get testResults(): IObservableValue<TestResult[]> {
        const ret = observable.box([] as TestResult[], {deep: false});
        if (!store.isSignedIn) {
            return ret;
        }
        const authRes = store.googleAuth!.currentUser.get().getAuthResponse();
        // artificially depends on the timestamp
        // so the API call can be treated as a pure function of f(timestamp, request)
        this.timestamp;
        fetch(
            'https://staging.results.api.cr.dev/prpc/luci.resultdb.rpc.v1.ResultDB/QueryTestResults',
            {
                method: 'POST',
                headers: {
                    accept: 'application/json',
                    'content-type': 'application/json',
                    authorization: `${(authRes as any).token_type} ${authRes.access_token}`,
                },
                body: JSON.stringify({
                    "invocations": [this.invocationName]
                }),
            },
        ).then(res => res.text())
            .then(res => JSON.parse(res.slice(4)).testResults as TestResult[])
            .then(res => ret.set(res));
        return ret;
    }

    @computed
    private get testExonerations(): IObservableValue<TestExoneration[]> {
        const ret = observable.box([] as TestExoneration[], {deep: false});
        if (!store.isSignedIn) {
            return ret;
        }
        const authRes = store.googleAuth!.currentUser.get().getAuthResponse();
        // artificially depends on the timestamp
        // so the API call can be treated as a pure function of f(timestamp, request)
        this.timestamp;
        fetch(
            'https://staging.results.api.cr.dev/prpc/luci.resultdb.rpc.v1.ResultDB/QueryTestExonerations',
            {
                method: 'POST',
                headers: {
                    accept: 'application/json',
                    'content-type': 'application/json',
                    authorization: `${(authRes as any).token_type} ${authRes.access_token}`,
                },
                body: JSON.stringify({
                    "invocations": [this.invocationName]
                }),
            },
        ).then(res => res.text())
            .then(res => JSON.parse(res.slice(4)).testExonerations as TestExoneration[])
            .then(res => ret.set(res));
        return ret;
    }


    @computed
    private get filteredTestResults() {
        return this.testResults.get().filter(this.filter);
    }

    @computed
    private get paginatedFilteredTestResults() {
        return this.filteredTestResults.slice(this.startIndex, this.startIndex + this.pageSize);
    }

    @computed
    private get resultStats() {
        const expected = this.testResults.get().filter((result) => result.expected).length;
        return {
            expected,
            unexpected: this.testResults.get().length - expected,
            exonerated: this.testExonerations.get().length,
            flaky: 0,
        }
    }

    protected render() {
        return html`
        <div id="test-invocation-summary" class="test-result-stat-card mdc-card mdc-card--outlined">
            <div id="invocation-name">Invocation Name: ${this.invocation.get()?.name || ""}</div>
            <div id="invocation-state">
                State: ${this.invocation.get()?.state || ""}
                <mwc-icon-button
                    @click=${() => this.timestamp = Date.now()}
                    id="refresh-icon"
                    class=${classMap({
                        hidden: this.invocation.get()?.state === "FINALIZED",
                        'inline-icon': true,
                    })}
                    icon="refresh"
                ></mwc-icon-button>
            </div>
            <div id="test-result-stats">
                <div class="test-result-stat">Unexpected: ${this.resultStats.unexpected}</div>
                <div class="test-result-stat">
                    <mwc-icon class="inline-icon" title="Only includes tests appear to be flaky in this invocation.">info</mwc-icon>
                    Flaky: ${this.resultStats.flaky}
                </div>
                <div class="test-result-stat">Exonerated: ${this.resultStats.exonerated}</div>
                <div class="test-result-stat">Expected: ${this.resultStats.expected}</div>
            </div>
        </div>
        <milo-test-result-filters
            id="test-result-filter"
            .onFilterChange=${(filter: TestResultFilter) => this.filter = filter}
        ></milo-test-result-filters>
        <milo-paginator
            .startIndex=${this.startIndex}
            .pageSize=${this.pageSize}
            .total=${this.filteredTestResults.length}
            .onStartIndexUpdated=${(startIndex: number) => this.startIndex = startIndex}
        >
        </milo-paginator>
        <div id="test-result-view">
            ${repeat(this.paginatedFilteredTestResults, (testResult) => html`
            <milo-test-result-entry
                .testResult=${testResult}
                .expanded=${this.expandedTestResults.get(testResult.name) || false}
                .onHeaderClicked=${() => this.expandedTestResults.set(testResult.name, !this.expandedTestResults.get(testResult.name))}
            ></milo-test-result-entry>
            `)}
        </div>
        <milo-paginator
            .startIndex=${this.startIndex}
            .pageSize=${this.pageSize}
            .total=${this.filteredTestResults.length}
            .onStartIndexUpdated=${(startIndex: number) => this.startIndex = startIndex}
        >
        </milo-paginator>
        ${this.invocation.get() && html`
        <milo-test-invocation-additional-info
            id="invocation-additional-info"
            .invocation=${this.invocation.get()}
        ></milo-test-invocation-additional-info>
        `}
        `;
    }

    constructor() {
        super();
        (this.shadowRoot as any).adoptedStyleSheets = [
            mdcCardStyle,
            ...(this.shadowRoot as any).adoptedStyleSheets,
        ];
    }

    static styles = css`
        :host {
            display: grid;
            grid-gap: 5px;
        }
        #refresh-icon {
            color: #03a9f4;
            --mdc-icon-button-size: 1em;
            --mdc-icon-size: 1em;
            vertical-align: middle;
        }
        #test-invocation-summary {
            display: grid;
            grid-gap: 1px;
            grid-template-rows: repeat(3, 24px);
        }
        #test-result-view {
            display: grid;
            grid-gap: 5px;
        }
        .mdc-card {
            padding: 5px;
        }
        #test-result-stats {
            display: flex;
            flex-direction: row;
        }
        .test-result-stat {
            flex-grow: 1;
        }
        .inline-icon {
            --mdc-icon-size: 1em;
            vertical-align: middle;
        }
        .hidden {
            display: none;
        }
        `;
}
