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

import { parseTestResultName } from './utils';

describe('parseTestResultName', () => {
  it('should parse valid test result name', () => {
    const name = parseTestResultName(
      'invocations/inv%2fid/tests/test%2fid/results/result%2fid',
    );
    expect(name).toEqual({
      invocationId: 'inv%2fid',
      testId: 'test/id',
      resultId: 'result%2fid',
    });
  });

  it('should reject invalid test result name', () => {
    expect(() =>
      parseTestResultName(
        'invocatios/inv%2fid/tests/test%2fid/results/result%2fid',
      ),
    ).toThrow('invalid test result name');
  });
});
