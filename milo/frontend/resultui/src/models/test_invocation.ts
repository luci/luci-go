/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// tslint:disable: max-classes-per-file

import { PrpcClient } from '@chopsui/prpc-client';
import deepEqual from 'deep-equal';
import { action, computed, observable } from 'mobx';

import { Invocation, QueryTestExonerationsResponse, QueryTestResultsResponse, TestExoneration, TestResult, Variant } from '../services/resultdb';


export type TestOutput = TestResult | TestExoneration;

export function isTestResult(testOutput: TestOutput): testOutput is TestResult {
    return (testOutput as TestResult).resultId !== undefined;
}

/***
 * view-model of TestInvocationElement
 * Starts loading the invocation, all test results and all exonerations of the invocation when initialized.
 */
export class TestInvocation {
    // null when invocation is not loaded yet.
    @computed
    get invocation() { return this._invocation; }
    @observable.ref
    private _invocation: Invocation | null = null;

    // All currently loaded test results of the invocation.
    @computed
    get testResults() { return this._testResults; }
    @observable.shallow
    private _testResults: TestResult[] = [];

    // All currently loaded exonerations of the invocation.
    @computed
    get testExonerations() { return this._testExonerations; }
    @observable.shallow
    private _testExonerations: TestExoneration[] = [];

    @computed
    get tests() {
        const ret: Test[] = [];

        // Group test results and exonerations by ID.
        // Constraint: this.testResults & this.testExonerations are sorted by testId following the same rule.
        let lastTestId = '';
        let testResultPivot = 0;
        let testExonerationPivot = 0;
        while (testResultPivot < this.testResults.length || testExonerationPivot < this.testExonerations.length) {
            if (testResultPivot < this.testResults.length && this.testResults[testResultPivot].testId === lastTestId) {
                ret[ret.length - 1].addResult(this.testResults[testResultPivot]);
                testResultPivot += 1;
            } else if (testExonerationPivot < this.testExonerations.length && this.testExonerations[testExonerationPivot].testId === lastTestId) {
                ret[ret.length - 1].addExoneration(this.testExonerations[testExonerationPivot]);
                testExonerationPivot += 1;
            } else {
                lastTestId = testResultPivot < this.testResults.length ? this.testResults[testResultPivot].testId : this.testExonerations[testExonerationPivot].testId;
                ret.push(new Test(lastTestId));
            }
        }
        return ret;
    }


    @computed
    get resultStats() {
        const expected = this.testResults.filter((result) => result.expected).length;
        return {
            expected,
            unexpected: this.testResults.length - expected,
            exonerated: this.testExonerations.length,
        };
    }

    @computed
    get testIdCommonPrefixPath() {
        if (this.tests.length === 0) {
            return '';
        }
        const testIds = this.tests.map((t) => t.id);
        let commonPrefixLength = 0;
        let commonPrefixPathLength = 0;
        while (testIds.every((id) => id[commonPrefixLength] === testIds[0][commonPrefixLength])) {
            commonPrefixLength += 1;
            if (!/[a-zA-Z_-]/.test(testIds[0][commonPrefixLength-1])) {
                commonPrefixPathLength = commonPrefixLength;
            }
        }
        return testIds[0].slice(0, commonPrefixPathLength);
    }

    constructor(readonly invocationName: string, private readonly resultDbPrpcClient: PrpcClient) {
        this.loadInvocation();
        this.loadTestResults();
        this.loadTestExonerations();
    }

    private async loadInvocation() {
        const res = await this.resultDbPrpcClient!.call(
            'luci.resultdb.rpc.v1.ResultDB',
            'GetInvocation',
            {name: this.invocationName},
        ) as Invocation;
        action(() => this._invocation = res)();
    }

    // Page tokens.
    // undefined when no page is loaded yet.
    // null when the last page is reached.
    private testResultPageToken?: string | null;
    private testExonerationPageToken?: string | null;

    private async loadTestResults() {
        if (this.testResultPageToken === null) {
            return;
        }
        const pageToken = this.testResultPageToken;
        this.testResultPageToken = null;
        const res = await this.resultDbPrpcClient!.call(
            'luci.resultdb.rpc.v1.ResultDB',
            'QueryTestResults',
            {invocations: [this.invocationName], pageToken},
        ) as QueryTestResultsResponse;
        action(() => this._testResults.push(...res.testResults))();
        this.testResultPageToken = res.nextPageToken || null;
        await this.loadTestResults();
    }

    private async loadTestExonerations() {
        if (this.testExonerationPageToken === null) {
            return;
        }
        const pageToken = this.testExonerationPageToken;
        this.testExonerationPageToken = null;
        const res = await this.resultDbPrpcClient!.call(
            'luci.resultdb.rpc.v1.ResultDB',
            'QueryTestExonerations',
            {invocations: [this.invocationName], pageToken},
        ) as QueryTestExonerationsResponse;
        action(() => this._testExonerations.push(...res.testExonerations))();
        this.testExonerationPageToken = res.nextPageToken || null;
        await this.loadTestExonerations();
    }
}


class Test {
    @observable.shallow
    readonly variants: TestVariant[] = [];

    constructor(readonly id: string) {}

    addResult(r: TestResult) {
        let variant = this.variants.find((v) => deepEqual(v.variant, r.variant));
        if (variant === undefined) {
            variant = new TestVariant(r.variant!);
            this.variants.push(variant);
        }
        variant.results.push(r);
    }

    addExoneration(r: TestExoneration) {
        let variant = this.variants.find((v) => deepEqual(v.variant, r.variant));
        if (variant === undefined) {
            variant = new TestVariant(r.variant!);
            this.variants.push(variant);
        }
        variant.exonerations.push(r);
    }
}

export enum VariantStatus {
    Failed,
    Flaky,
    Passed,
}

class TestVariant {
    @computed
    get expectedResults() {
        return this.results.filter((result) => result.expected);
    }
    @computed
    get unexpectedResults() {
        return this.results.filter((result) => !result.expected);
    }
    @computed
    get status() {
        if (this.unexpectedResults.length !== 0) {
            if (this.expectedResults.length === 0) {
                return VariantStatus.Failed;
            } else {
                return VariantStatus.Flaky;
            }
        }
        return VariantStatus.Passed;
    }

    @observable.shallow
    readonly results: TestResult[] = [];

    @observable.shallow
    readonly exonerations: TestExoneration[] = [];

    constructor(readonly variant: Variant) {}
}
