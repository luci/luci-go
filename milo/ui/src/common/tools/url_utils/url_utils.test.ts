// Copyright 2023 The LUCI Authors.
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

import { TestIdentifier } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

import {
  generateTestInvestigateUrl,
  generateTestInvestigateUrlForLegacyInvocations,
  generateTestInvestigateUrlFromOld,
  getBuilderURLPath,
  getSwarmingBotListURL,
  getTestHistoryURLPath,
  setSingleQueryParam,
} from './url_utils';

describe('getBuilderURLPath', () => {
  test('should encode the builder', () => {
    const url = getBuilderURLPath({
      project: 'testproject',
      bucket: 'testbucket',
      builder: 'test builder',
    });
    expect(url).toStrictEqual(
      '/ui/p/testproject/builders/testbucket/test%20builder',
    );
  });
});

describe('getTestHisotryURLPath', () => {
  test('should encode the test ID', () => {
    const url = getTestHistoryURLPath('testproject', 'test/id');
    expect(url).toStrictEqual('/ui/test/testproject/test%2Fid');
  });
});

describe('getSwarmingBotListURL', () => {
  test('should support multiple dimensions', () => {
    const url = getSwarmingBotListURL('chromium-swarm-dev.appspot.com', [
      'cpu:x86-64',
      'os:Windows-11',
    ]);
    expect(url).toStrictEqual(
      'https://chromium-swarm-dev.appspot.com/botlist?f=cpu%3Ax86-64&f=os%3AWindows-11',
    );
  });
});

describe('setSingleQueryParam', () => {
  it('should set parameter string value', () => {
    const parameterString = 'a=b&c=d';
    const updatedParams = setSingleQueryParam(parameterString, 'e', 'f');
    expect(updatedParams.toString()).toEqual('a=b&c=d&e=f');
  });

  it('should update parameter string value', () => {
    const parameterString = 'a=b&c=d';
    const updatedParams = setSingleQueryParam(parameterString, 'a', 'f');
    expect(updatedParams.toString()).toEqual('a=f&c=d');
  });
});

describe('generateTestInvestigateUrlForLegacyInvocations', () => {
  it('should construct a basic structured legacy URL', () => {
    const url = generateTestInvestigateUrlForLegacyInvocations(
      'inv-123',
      'test.name.basic',
      'abc',
    );
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-123/modules/legacy/schemes/legacy/variants/abc/cases/test.name.basic?invMode=legacy',
    );
  });

  it('should correctly URL-encode slashes in the testId', () => {
    const url = generateTestInvestigateUrlForLegacyInvocations(
      'inv-456',
      'test.suite/test.name/with/slashes',
      'def',
    );
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-456/modules' +
        '/legacy/schemes/legacy/variants/def/cases/test.suite%2Ftest.name%2Fwith%2Fslashes?invMode=legacy',
    );
  });

  it('should add extra query params', () => {
    const url = generateTestInvestigateUrlForLegacyInvocations(
      'inv-789',
      'Test: My Test!',
      'ghi',
    );
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-789/modules' +
        '/legacy/schemes/legacy/variants/ghi/cases/Test%3A%20My%20Test!?invMode=legacy',
    );
  });
});

describe('generateTestInvestigateUrl (from TestIdentifier)', () => {
  const baseStructuredId = TestIdentifier.fromPartial({
    moduleName: 'my-module',
    moduleScheme: 'my-scheme',
    moduleVariantHash: 'my-variant',
    caseName: 'my.test.case',
  });

  it('should generate a URL with full structure', () => {
    const testId = TestIdentifier.fromPartial({
      ...baseStructuredId,
      coarseName: 'c1',
      fineName: 'f1',
    });
    const url = generateTestInvestigateUrl('inv-full', testId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-full/modules' +
        '/my-module/schemes/my-scheme/variants/my-variant/coarse/c1/fine/f1/cases/my.test.case',
    );
  });

  it('should generate a URL with coarse only', () => {
    const testId = TestIdentifier.fromPartial({
      ...baseStructuredId,
      coarseName: 'c1',
    });
    const url = generateTestInvestigateUrl('inv-coarse', testId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-coarse/modules' +
        '/my-module/schemes/my-scheme/variants/my-variant/coarse/c1/cases/my.test.case',
    );
  });

  it('should generate a URL with fine only', () => {
    const testId = TestIdentifier.fromPartial({
      ...baseStructuredId,
      fineName: 'f1',
    });
    const url = generateTestInvestigateUrl('inv-fine', testId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-fine/modules/my-module/schemes/my-scheme/variants/my-variant/fine/f1/cases/my.test.case',
    );
  });

  it('should generate a minimal structured URL', () => {
    const testId = TestIdentifier.fromPartial({ ...baseStructuredId });
    const url = generateTestInvestigateUrl('inv-minimal', testId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-minimal/modules/my-module/schemes/my-scheme/variants/my-variant/cases/my.test.case',
    );
  });

  it('should correctly encode the caseName', () => {
    const testId = TestIdentifier.fromPartial({
      ...baseStructuredId,
      caseName: 'my.test/case with:spaces',
    });
    const url = generateTestInvestigateUrl('inv-encoded', testId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-encoded/modules' +
        '/my-module/schemes/my-scheme/variants/my-variant/cases/my.test%2Fcase%20with%3Aspaces',
    );
  });

  it('should correctly route legacy tests to the pluralized legacy URL generator', () => {
    const legacyTestId = TestIdentifier.fromPartial({
      moduleName: 'legacy',
      moduleScheme: 'legacy',
      caseName: 'this/is/a/legacy/id',
      moduleVariantHash: 'legacy-hash',
    });
    const url = generateTestInvestigateUrl('inv-legacy', legacyTestId);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-legacy/modules' +
        '/legacy/schemes/legacy/variants/legacy-hash/cases/this%2Fis%2Fa%2Flegacy%2Fid?invMode=legacy',
    );
  });
});

describe('generateTestInvestigateUrlFromOld', () => {
  it('should convert a basic legacy URL', () => {
    const oldUrl =
      '/ui/test-investigate/invocations/inv123/tests/test.name.basic/variants/abc';
    const newUrl =
      '/ui/test-investigate/invocations/inv123/modules/legacy/schemes/legacy/variants/abc/cases/test.name.basic?invMode=legacy';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(newUrl);
  });

  it('should preserve URL-encoded characters in the testId', () => {
    const oldUrl =
      '/ui/test-investigate/invocations/inv-xyz/tests/test.suite%2Ftest.name/variants/def';
    const newUrl =
      '/ui/test-investigate/invocations/inv-xyz/modules/legacy/schemes/legacy/variants/def/cases/test.suite%2Ftest.name?invMode=legacy';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(newUrl);
  });

  it('should append invMode=legacy to a URL with existing query params', () => {
    const oldUrl =
      '/ui/test-investigate/invocations/inv123/tests/test.name/variants/abc?foo=bar&baz=qux';
    const newUrl =
      '/ui/test-investigate/invocations/inv123/modules/legacy/schemes/legacy/variants/abc/cases/test.name?invMode=legacy&foo=bar&baz=qux';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(newUrl);
  });

  it('should return the original URL if it does not match', () => {
    const oldUrl = '/ui/p/chromium/builds/b12345';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(oldUrl);
  });

  it('should not match an incomplete legacy URL', () => {
    const oldUrl = '/ui/test-investigate/invocations/inv123/tests/test.name';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(oldUrl);
  });

  it('should not match the URL if it is missing the /test-investigate/ prefix', () => {
    const oldUrl = '/ui/invocations/inv123/tests/test.name/variants/abc';
    expect(generateTestInvestigateUrlFromOld(oldUrl)).toBe(oldUrl);
  });
});
