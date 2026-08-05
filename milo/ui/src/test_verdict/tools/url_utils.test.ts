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

import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import {
  getBlamelistUrl,
  getBlamelistUrlFromVariantBranch,
  toParsedTestVariantBranchName,
} from './url_utils';

describe('url_utils', () => {
  const fullBranch = TestVariantBranch.fromPartial({
    project: 'chromium',
    testId: 'ninja://test_target.TestCase',
    variantHash: 'v1234',
    refHash: 'r5678',
  });

  describe('getBlamelistUrl', () => {
    it('creates blamelist URL without commit position', () => {
      expect(
        getBlamelistUrl({
          project: 'chromium',
          testId: 'ninja://test_target.TestCase',
          variantHash: 'v1234',
          refHash: 'r5678',
        }),
      ).toBe(
        '/ui/labs/p/chromium/tests/ninja%3A%2F%2Ftest_target.TestCase/variants/v1234/refs/r5678/blamelist',
      );
    });

    it('creates blamelist URL with commit position hash', () => {
      expect(
        getBlamelistUrl(
          {
            project: 'chromium',
            testId: 'test.id',
            variantHash: 'v1234',
            refHash: 'r5678',
          },
          '100',
        ),
      ).toBe(
        '/ui/labs/p/chromium/tests/test.id/variants/v1234/refs/r5678/blamelist#CP-100',
      );
    });
  });

  describe('toParsedTestVariantBranchName', () => {
    it('returns undefined if tvb is null or undefined', () => {
      expect(toParsedTestVariantBranchName(null)).toBeUndefined();
      expect(toParsedTestVariantBranchName(undefined)).toBeUndefined();
    });

    it('returns undefined if tvb is missing refHash, project, or testId', () => {
      expect(
        toParsedTestVariantBranchName(
          TestVariantBranch.fromPartial({
            project: 'chromium',
            testId: 'test.id',
          }),
        ),
      ).toBeUndefined();
      expect(
        toParsedTestVariantBranchName(
          TestVariantBranch.fromPartial({
            refHash: 'r5678',
            testId: 'test.id',
          }),
        ),
      ).toBeUndefined();
      expect(
        toParsedTestVariantBranchName(
          TestVariantBranch.fromPartial({
            refHash: 'r5678',
            project: 'chromium',
          }),
        ),
      ).toBeUndefined();
    });

    it('parses a full branch directly', () => {
      expect(toParsedTestVariantBranchName(fullBranch)).toEqual({
        project: 'chromium',
        testId: 'ninja://test_target.TestCase',
        variantHash: 'v1234',
        refHash: 'r5678',
      });
    });
  });

  describe('getBlamelistUrlFromVariantBranch', () => {
    it('returns undefined when branch is missing', () => {
      expect(getBlamelistUrlFromVariantBranch(null)).toBeUndefined();
      expect(getBlamelistUrlFromVariantBranch(undefined)).toBeUndefined();
    });

    it('creates URL from full branch', () => {
      expect(getBlamelistUrlFromVariantBranch(fullBranch)).toBe(
        '/ui/labs/p/chromium/tests/ninja%3A%2F%2Ftest_target.TestCase/variants/v1234/refs/r5678/blamelist',
      );
    });

    it('creates URL with commit position', () => {
      expect(getBlamelistUrlFromVariantBranch(fullBranch, '200')).toBe(
        '/ui/labs/p/chromium/tests/ninja%3A%2F%2Ftest_target.TestCase/variants/v1234/refs/r5678/blamelist#CP-200',
      );
    });
  });
});
