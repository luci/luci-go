import { MobxLitElement } from '@adobe/lit-mobx';
import { html, css, customElement } from 'lit-element';
import { computed, observable, IObservableValue } from 'mobx';
import { repeat } from 'lit-html/directives/repeat';
import { classMap } from 'lit-html/directives/class-map';
import '@material/mwc-icon';
import '@material/mwc-icon-button';
import '@material/mwc-button';
import * as _ from 'lodash';

import { TestResult, Invocation, TestExoneration, InvocationState } from '../../models/resultdb';
import { TestResultFilter } from './test-result-filters';
import { store } from '../../store';
import './test-invocation-details';
import './test-result-filters';
import './test-entry';
import '../../lib/components/paginator';
import './test-result-tree';
import moment from 'moment';

const INVOCATION_STATE_DISPLAY_MAP = {
    [InvocationState.Unspecified]: 'unspecified',
    [InvocationState.Active]: 'active',
    [InvocationState.Finalizing]: 'finalizing',
    [InvocationState.Finalized]: 'finalized',
};

const INVOCATION_STATE_CLASS_MAP = {
    [InvocationState.Unspecified]: 'unspecified',
    [InvocationState.Active]: 'active',
    [InvocationState.Finalizing]: 'finalizing',
    [InvocationState.Finalized]: 'finalized',
};

interface QueryTestResultsRes {
    testResults: TestResult[],
    nextPageToken?: string,
}
class TestResultsReq {
    @observable.ref
    private _testResults: TestResult[] = [];

    @computed
    public get testResults() {
        return this._testResults.map((tr, i) => {
            if (i < 15) {
                return tr;
            } else {
                return {
                    ...tr,
                    testId: 'gpu_tests.pixel_integration_test.PixelIntegrationRegression.' + tr.testId.slice('gpu_tests.pixel_integration_test.PixelIntegrationTest.'.length),
                }
            }
        })
    }

    @observable.ref
    public pageToken: string | null = null;

    constructor(private invocationName: string) {

        store.resultDbPrpcClient!.call(
            'luci.resultdb.rpc.v1.ResultDB',
            'QueryTestResults',
            {
                invocations: [this.invocationName],
                predicate: {
                    expectancy: 1,
                },
                pageSize: 20,
            },
        ).then((res: any) => {
            const results = res.testResults as TestResult[];
            this._testResults = this._testResults.concat(results);
            this.pageToken = res.nextPageToken;
        });
    }

    public async loadNext() {
        if (!this.pageToken) {
            return;
        }
        const pageToken = this.pageToken;
        this.pageToken = null;

        const res = await store.resultDbPrpcClient!.call(
            'luci.resultdb.rpc.v1.ResultDB',
            'QueryTestResults',
            {
                invocations: [this.invocationName],
                predicate: {
                    expectancy: 1,
                },
                pageToken,
                pageSize: 20,
            },
        ) as QueryTestResultsRes;
        
        const results = res.testResults as TestResult[];
        this.pageToken = res.nextPageToken || null;
        this._testResults = this._testResults.concat(results);
        return results;
    }
}

@customElement('tr-test-invocation-view')
export class TestInvocationView extends MobxLitElement {
    @observable.ref
    private invocationName: string = "";

    @observable.ref
    private timestamp: number = Date.now();

    @observable.ref
    private filter: TestResultFilter = () => true;

    @observable.ref
    private menuExpanded: boolean = false;

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
    private get testResultsReq(): TestResultsReq | null {
        if (!store.isSignedIn) {
            return null;
        }
        // artificially depends on the timestamp
        // so the API call can be treated as a pure function of f(timestamp, request)
        this.timestamp;
        return new TestResultsReq(this.invocationName);
    }

    @computed
    private get testResultsByTestId() {
        const groupedTestResults: [string, TestResult[]][] = [];
        let lastTestId = '';
        for (const testResult of this.filteredTestResults) {
            if (testResult.testId === lastTestId) {
                groupedTestResults[groupedTestResults.length - 1][1].push(testResult);
            } else {
                lastTestId = testResult.testId;
                groupedTestResults.push([lastTestId, [testResult]]);
            }
        }

        return groupedTestResults;
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
        return this.testResultsReq?.testResults.filter(this.filter) || [];
    }

    @computed
    private get resultStats() {
        const expected = this.testResultsReq?.testResults.filter((result) => result.expected).length || 0;
        return {
            expected,
            unexpected: (this.testResultsReq?.testResults.length || 0) - expected,
            exonerated: this.testExonerations.get().length,
        }
    }

    @computed
    private get testIdCommonPrefix() {
        const testIds = this.filteredTestResults.map((tr) => tr.testId);
        let commonPrefix = testIds[0] || '';
        for (const testId of testIds) {
            if (testId.startsWith(commonPrefix)) {
                continue;
            }
            let nextCommonLength = 0;
            for (let i = 0; i < commonPrefix.length; ++i) {
                if (testId[i] !== commonPrefix[i]) {
                    break;
                }
                if (!/\w|-/.test(testId[i])) {
                    nextCommonLength = i + 1;
                }
            }
            commonPrefix = testId.slice(0, nextCommonLength);
        }
        return commonPrefix;
    }


    private renderInvocationState() {
        const invocation = this.invocation.get()!;
        if (invocation.finalizeTime) {
            return html`
            <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
            at ${moment(invocation.finalizeTime).format('YYYY-MM-DD HH:mm:ss')}
            `
        }

        return html`
        <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
        since ${moment(invocation.createTime).format('YYYY-MM-DD HH:mm:ss')}
        `
    }

    protected render() {
        const invocation = this.invocation.get();
        if (invocation === null) {
            return html`<div>Loading</div>`;
        }

        return html`
        <div id="page-head" style="padding-left: 5px;">
            <img style="display: inline-block; width: 50px; vertical-align: middle;" src="/assets/chromium-icon.png"/>
            <span style="font-size: 32px; vertical-align: middle;">LUCI Test Results</span>
        </div>
        <div id="test-invocation-summary">
            <span>Invocation: ${invocation.name.slice('invocations/'.length)}</span>
            <span class="badge unexpected">${this.resultStats.unexpected} Unexpected</span>
            <span class="badge exonerated">${this.resultStats.exonerated} Exonerated</span>
            <span class="badge expected">${this.resultStats.expected} Expected</span>
            <span style="float: right">${this.renderInvocationState()}</span>
        </div>
        <tr-test-invocation-details
            id="invocation-details"
            .invocation=${invocation}
        ></tr-test-invocation-details>
        <div id="main" class=${classMap({'show-menu': this.menuExpanded})}>
            <div id="left-panel">
                <tr-test-result-filters
                    id="test-result-filter"
                    .onFilterChange=${(filter: TestResultFilter) => this.filter = filter}
                    .testResults=${this.testResultsReq?.testResults || []}
                    .testExonerations=${this.testExonerations.get()}
                ></tr-test-result-filters>
                <tr-test-result-tree
                    .branchName=""
                    .pathName=""
                    .testResults=${this.filteredTestResults}
                    .depth=${-1}
                ></tr-test-result-tree>
            </div>
            <div id="test-result-view">
                <div id="test-result-header">
                    <div
                        style="display: inline-table; height: 100%;"
                        @click=${() => this.menuExpanded = !this.menuExpanded}
                    >
                        <mwc-icon style="vertical-align: middle; display: table-cell">menu</mwc-icon>
                    </div>
                    <span style="vertical-align: middle; display: inline-table; height: 100%; margin-left: 5px;">${this.testIdCommonPrefix}</span>
                    <span style="float: right">
                        <mwc-button label="Expand All"></mwc-button>
                        <mwc-button label="Collapse All"></mwc-button>
                    </span>
                </div>
                <div id="test-result-content">
                    ${repeat(this.testResultsByTestId, ([testId]) => testId, ([testId, testResults], i) => html`
                    <tr-test-entry
                        .testId=${testId.slice(this.testIdCommonPrefix.length)}
                        .testResults=${testResults}
                        .prevTestId=${((this.testResultsByTestId[i-1] || [])[0] || '').slice(this.testIdCommonPrefix.length)}
                    ></tr-test-entry>
                    `)}
                    <div>
                        <span>Showing ${this.filteredTestResults.length}/${this.filteredTestResults.length + 10} test results.</span>
                        <span
                            id="load-more"
                            class=${classMap({hidden: !this.testResultsReq?.pageToken})}
                            @click=${() => this.testResultsReq?.loadNext()}
                        >
                            Load More
                        </span>
                    </div>
                </div>
            </div>
        </div>
        `;
    }

    static styles = css`
        :host {
            display: grid;
            grid-gap: 5px;
            grid-template-rows: auto auto auto 1fr;
        }
        #refresh-icon {
            color: #03a9f4;
            --mdc-icon-button-size: 1em;
            --mdc-icon-size: 1em;
            vertical-align: middle;
        }

        .badge {
            border: 1px solid;
            border-radius: 5px;
            padding: 0 2px 0 2px;
        }
        .badge.unexpected {
            color: rgb(210, 63, 49);
            border-color: rgb(210, 63, 49);
        }
        .badge.exonerated {
            color: #ffc107;
            border-color: #ffc107;
        }
        .badge.expected {
            color: rgb(51, 172, 113);
            border-color: rgb(51, 172, 113);
        }

        #main {
            display: grid;
            grid-template-columns: 350px 1fr;
            grid-template-rows: 1fr;
            grid-template-areas: "test-result-view test-result-view";
            border-top: 2px solid #DDDDDD;
        }
        #main.show-menu {
            grid-template-areas: "left-panel test-result-view";
        }
        #left-panel {
            grid-area: left-panel;
            display: none;
            height: 100%;
            border-right: 2px solid #DDDDDD;
        }
        #main.show-menu #left-panel {
            display: block;
        }
        #test-result-view {
            grid-area: test-result-view;
            display: grid;
            grid-template-rows: 32px 1fr;
        }
        #test-result-header {
            background: #DDDDDD;
        }
        #test-result-content > * {
            margin: 5px 0 5px 0;
            display: block;
        }
        #test-invocation-summary {
            border-bottom: solid red;
            padding: 5px;
        }

        .inline-icon {
            --mdc-icon-size: 1em;
            vertical-align: middle;
        }
        #load-more {
            color: blue;
        }

        .hidden {
            display: none;
        }
        `;
}
