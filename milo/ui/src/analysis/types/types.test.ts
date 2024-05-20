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

import { ParsedTestVariantBranchName } from './types';

describe('ParsedTestVariantBranchName', () => {
  it('can parse test variant branch name', () => {
    const parsed = ParsedTestVariantBranchName.fromString(
      'projects/proj/tests/test%2fid/variants/vhash/refs/refhash',
    );
    expect(parsed).toEqual<ParsedTestVariantBranchName>({
      project: 'proj',
      testId: 'test/id',
      variantHash: 'vhash',
      refHash: 'refhash',
    });
  });

  it('can stringify test variant branch name', () => {
    const stringified = ParsedTestVariantBranchName.toString({
      project: 'proj',
      testId: 'test/id',
      variantHash: 'vhash',
      refHash: 'refhash',
    });
    expect(stringified).toEqual(
      'projects/proj/tests/test%2Fid/variants/vhash/refs/refhash',
    );
  });

  it('rejects invalid test variant branch name', () => {
    expect(() =>
      ParsedTestVariantBranchName.fromString(
        'projects/tests/test%2Fid/variants/vhash/refs/refhash',
      ),
    ).toThrowErrorMatchingInlineSnapshot(
      '"invalid TestVariantBranchName: projects/tests/test%2Fid/variants/vhash/refs/refhash"',
    );
  });
});
