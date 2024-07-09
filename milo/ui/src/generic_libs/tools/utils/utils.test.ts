// Copyright 2020 The LUCI Authors.
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

import { URLExt } from './utils';

describe('URLExt', () => {
  let url: URLExt;
  beforeEach(() => {
    url = new URLExt('https://example.com/path?key1=val1&key2=val2');
  });

  test('should set search params correctly', async () => {
    const newUrlStr = url
      .setSearchParam('key2', 'newVal2')
      .setSearchParam('key3', 'newVal3')
      .toString();
    expect(newUrlStr).toStrictEqual(
      'https://example.com/path?key1=val1&key2=newVal2&key3=newVal3',
    );
  });

  test('should remove matched search params correctly', async () => {
    const newUrlStr = url
      .removeMatchedParams({ key1: 'val1', key2: 'val', key3: 'val3' })
      .toString();
    expect(newUrlStr).toStrictEqual('https://example.com/path?key2=val2');
  });

  test('should not remove search params when multiple values are specified', async () => {
    const url = new URLExt('https://example.com/path?key1=val1&key1=val2');
    const newUrlStr = url.removeMatchedParams({ key1: 'val1' }).toString();
    expect(newUrlStr).toStrictEqual(
      'https://example.com/path?key1=val1&key1=val2',
    );
  });
});
