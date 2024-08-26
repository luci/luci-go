// Copyright 2024 The LUCI Authors.
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

import '@/proto_utils/duration_patch';

import * as fs from 'node:fs';

import {
  Reporter,
  Test,
  TestCaseResult,
  Config,
  ReporterContext,
} from '@jest/reporters';

import { PrpcClient } from '@/generic_libs/tools/prpc_client';
import {
  ReportTestResultsRequest,
  SinkClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/sink/proto/v1/sink.pb';

import { ToSinkResultContext, toSinkResult } from './convert';

interface ResultSinkCtx {
  readonly address: string;
  readonly auth_token: string;
}

interface LUCIContext {
  readonly result_sink?: ResultSinkCtx;
}

interface ResultDBReporterOptions {
  /**
   * The source repository that the jest unit tests are hosted in.
   *
   * `repo` and `directory` are added as prefix to the test ID.
   */
  readonly repo: string;
  /**
   * The directory that test jest unit tests are hosted in.
   *
   * `repo` and `directory` are added as prefix to the test ID.
   */
  readonly directory: string;
  /**
   * The delimiter used to join the repository, directory, test suite titles,
   * and test title when constructing test ID and test Name.
   *
   * The delimiter should be picked to avoid potential test ID collision.
   * Otherwise test results from multiple test cases might get grouped under
   * a single test.
   *
   * When the delimiter is ' > ':
   *  * the test name has the following format:
   *    "parent test suite title > test suite title > test case title".
   *  * the test ID has the following format:
   *    "[repo host] > [directory]/path/to/test/file > [test-name]".
   */
  readonly delimiter: string;
  /**
   * The bug component ID of the unit tests. When specified, this will be
   * recorded at
   * `TestResult.test_metadata.bug_component.issue_tracker.component_id`.
   */
  readonly bugComponentId?: string;
}

/**
 * An **experimental** Jest Reporter that uploads Jest test results to ResultDB
 * *when the tests are run with a result sink context* (i.e. run with
 * `rdb stream`).
 *
 * This is experimental. If you want to use this, please contact
 * chops-luci-test@google.com.
 */
export class ResultDBReporter implements Reporter {
  private readonly ctx: ToSinkResultContext;
  private readonly resultSink: SinkClientImpl | undefined;

  constructor(
    private readonly globalConfig: Config.GlobalConfig,
    opts: ResultDBReporterOptions | undefined,
    _reporterContext: ReporterContext,
  ) {
    if (!opts?.repo) {
      throw new Error('repo must be specified.');
    }
    if (!opts?.directory) {
      throw new Error('directory must be specified.');
    }
    if (!opts?.delimiter) {
      throw new Error('delimiter must be specified.');
    }
    this.ctx = {
      ...opts,
      stackTraceOpts: this.globalConfig,
    };

    const luciCtxFile = process.env['LUCI_CONTEXT'];
    const luciCtx = JSON.parse(
      luciCtxFile ? fs.readFileSync(luciCtxFile, { encoding: 'utf-8' }) : '{}',
    ) as LUCIContext;
    const sinkCtx = luciCtx.result_sink;
    if (sinkCtx) {
      this.resultSink = new SinkClientImpl(
        new PrpcClient({
          host: sinkCtx.address,
          getAuthToken: () => sinkCtx.auth_token,
          tokenType: 'ResultSink',
          insecure: true,
          fetchImpl: fetch,
        }),
      );
    }
  }

  async onTestCaseResult(test: Test, testCaseResult: TestCaseResult) {
    // Ensure that failing to upload test results to RDB does not prevent the
    // rest of the tests from executing.
    try {
      const req = ReportTestResultsRequest.fromPartial({
        testResults: [await toSinkResult(test, testCaseResult, this.ctx)],
      });
      await this.resultSink?.ReportTestResults(req);
    } catch (e) {
      // We need to log the error in builder.
      // eslint-disable-next-line no-console
      console.error('failed to report test results to result sink', e);
    }
  }
}
