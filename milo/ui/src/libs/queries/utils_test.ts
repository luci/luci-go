// Copyright 2022 The LUCI Authors.
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

import { expect } from '@jest/globals';

import { parseKeyValue } from './utils';

describe('parseKeyValue', () => {
  it('should parse key-value pair', () => {
    const [key, value] = parseKeyValue('key=value');
    expect(key).toStrictEqual('key');
    expect(value).toStrictEqual('value');
  });

  it("should work when there's no value", () => {
    const [key, value] = parseKeyValue('key');
    expect(key).toStrictEqual('key');
    expect(value).toStrictEqual(null);
  });

  it('should work when the value is empty', () => {
    const [key, value] = parseKeyValue('key=');
    expect(key).toStrictEqual('key');
    expect(value).toStrictEqual('');
  });

  it('should work when the key and value contain encoded characters', () => {
    const [key, value] = parseKeyValue('%25key%20=value+');
    expect(key).toStrictEqual('%key ');
    expect(value).toStrictEqual('value ');
  });

  it('should work when the key and value contain special characters', () => {
    const [key, value] = parseKeyValue('key=valuekey=real&value');
    expect(key).toStrictEqual('key');
    expect(value).toStrictEqual('valuekey=real&value');
  });
});
