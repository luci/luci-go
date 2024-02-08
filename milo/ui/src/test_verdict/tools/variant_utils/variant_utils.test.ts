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

import { getCriticalVariantKeys } from './variant_utils';

describe('getCriticalVariantKeys', () => {
  it('when all variants are the same', () => {
    const keys = getCriticalVariantKeys([
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
    ]);
    expect(keys).toEqual(['key1']);
  });

  it('when some variants are the different', () => {
    const keys = getCriticalVariantKeys([
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val3' } },
      { def: { key1: 'val1', key2: 'val2' } },
    ]);
    expect(keys).toEqual(['key2']);
  });

  it('when some variant defs has missing keys', () => {
    const keys = getCriticalVariantKeys([
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1', key2: 'val2' } },
      { def: { key1: 'val1' } },
    ]);
    expect(keys).toEqual(['key2']);
  });

  it('when some variant values always change together', () => {
    const keys = getCriticalVariantKeys([
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'linux-builder',
          os: 'linux',
        },
      },
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'macos-builder',
          os: 'macos',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'linux-builder',
          os: 'linux',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'macos-builder',
          os: 'macos',
        },
      },
    ]);
    expect(keys).toEqual(['builder', 'test_suite']);
  });

  it("when there are additional variant keys that don't matter", () => {
    const keys = getCriticalVariantKeys([
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'linux-builder',
          os: 'linux',
          a_param1: 'val1',
        },
      },
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'macos-builder',
          os: 'macos',
          a_param2: 'val2',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'linux-builder',
          os: 'linux',
          a_param3: 'val3',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'macos-builder',
          os: 'macos',
          a_param4: 'val4',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'macos-builder',
          os: 'macos',
          a_param5: 'val5',
        },
      },
    ]);
    // Having a_param4 is enough to uniquely identify all variants.
    expect(keys).toEqual(['builder', 'test_suite', 'a_param4']);
  });

  it('when there are multiple valid set of critical keys', () => {
    const keys = getCriticalVariantKeys([
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'linux-builder',
          os: 'linux',
          a_param: 'val1',
        },
      },
      {
        def: {
          test_suite: 'test-suite-1',
          builder: 'macos-builder',
          os: 'macos',
          a_param: 'val2',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'linux-builder',
          os: 'linux',
          a_param: 'val3',
        },
      },
      {
        def: {
          test_suite: 'test-suite-2',
          builder: 'macos-builder',
          os: 'macos',
          a_param: 'val4',
        },
      },
    ]);
    // Having a_param is enough to uniquely identify all variants.
    // But we prefer keys that are known to have special meanings.
    expect(keys).toEqual(['builder', 'test_suite']);
  });
});
