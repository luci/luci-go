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

import { getLongestCommonPrefix } from './string_utils';

describe('getLongestCommonPrefix', () => {
  it('should work with empty array', () => {
    expect(getLongestCommonPrefix([])).toEqual('');
  });

  it('should work with single string', () => {
    expect(getLongestCommonPrefix(['a_string'])).toEqual('a_string');
  });

  it('should work with multiple strings', () => {
    expect(
      getLongestCommonPrefix(['string1', 'string2', 'string3', 'string4']),
    ).toEqual('string');
  });

  it('prefix is a member', () => {
    expect(
      getLongestCommonPrefix(['stringabc', 'string', 'stringdd', 'stringz']),
    ).toEqual('string');
  });

  it('no shared prefix', () => {
    expect(getLongestCommonPrefix(['abc', 'abce', 'aad', 'ze'])).toEqual('');
  });

  it('same strings', () => {
    expect(getLongestCommonPrefix(['abc', 'abc', 'abc', 'abc'])).toEqual('abc');
  });

  it('has empty string', () => {
    expect(getLongestCommonPrefix(['', 'aaa', 'aaab', 'aaac'])).toEqual('');
  });
});
