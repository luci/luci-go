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

import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import {
  linkToBuild,
  linkToBuilder,
  linkToCommit,
  linkToCommitRange,
} from './link_constructors';

describe('Test link constructors', () => {
  test('getting link to build', () => {
    expect(linkToBuild('12345678987654321')).toStrictEqual({
      linkText: '12345678987654321',
      url: 'https://ci.chromium.org/b/12345678987654321',
    });
  });

  test('getting link to builder', () => {
    const mockBuilder: BuilderID = {
      project: 'mockProject',
      bucket: 'mockBucket',
      builder: 'mockBuilder',
    };
    expect(linkToBuilder(mockBuilder)).toStrictEqual({
      linkText: 'mockProject/mockBucket/mockBuilder',
      url: 'https://ci.chromium.org/p/mockProject/builders/mockBucket/mockBuilder',
    });
  });

  test('getting link to commit', () => {
    const mockCommit: GitilesCommit = {
      host: 'mockHost',
      project: 'mockProject',
      id: '123456abcdef',
      ref: 'ref/main',
      position: 321,
    };
    expect(linkToCommit(mockCommit)).toStrictEqual({
      linkText: '123456a',
      url: 'https://mockHost/mockProject/+/123456abcdef',
    });
  });

  test('getting link to commit range', () => {
    const firstMockCommit: GitilesCommit = {
      host: 'mockHost',
      project: 'mockProject',
      id: '123456abcdef',
      ref: 'ref/main',
      position: 321,
    };
    const secondMockCommit: GitilesCommit = {
      host: 'mockHost',
      project: 'mockProject',
      id: '987654fedcba',
      ref: 'ref/main',
      position: 654,
    };
    expect(linkToCommitRange(firstMockCommit, secondMockCommit)).toStrictEqual({
      linkText: '123456a ... 987654f',
      url: 'https://mockHost/mockProject/+log/123456abcdef..987654fedcba',
    });
  });
});
