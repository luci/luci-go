// Copyright 2021 The LUCI Authors.
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

import { assert } from 'chai';

import { attachTags, hasTags, TAG_SOURCE } from './tag';

const TAG_1 = {};
const TAG_2 = {};

describe('tag', () => {
  it('can attach a tag to an object', () => {
    const obj1 = {};
    const obj2 = {};
    attachTags(obj1, TAG_1);
    attachTags(obj2, TAG_2);

    assert.isTrue(hasTags(obj1, TAG_1));
    assert.isTrue(hasTags(obj2, TAG_2));

    assert.isFalse(hasTags(obj1, TAG_2));
    assert.isFalse(hasTags(obj2, TAG_1));
  });

  it('can attach multiple tags to an object', () => {
    const obj = {};

    attachTags(obj, TAG_1, TAG_2);

    assert.isTrue(hasTags(obj, TAG_1, TAG_2));
  });

  it('can detect tag attached to [TAG_SOURCE]', () => {
    const innerObj = {};
    const outerObj = {
      [TAG_SOURCE]: innerObj,
    };

    attachTags(innerObj, TAG_1);
    attachTags(outerObj, TAG_2);

    assert.isTrue(hasTags(innerObj, TAG_1));
    assert.isFalse(hasTags(innerObj, TAG_2));
    assert.isFalse(hasTags(innerObj, TAG_1, TAG_2));

    assert.isTrue(hasTags(outerObj, TAG_1));
    assert.isTrue(hasTags(outerObj, TAG_2));
    assert.isTrue(hasTags(outerObj, TAG_1, TAG_2));
  });

  it('calling attachTags multiple times should not override previous calls', () => {
    const obj = {};

    attachTags(obj, TAG_1);
    attachTags(obj, TAG_2);

    assert.isTrue(hasTags(obj, TAG_1, TAG_2));
  });
});
