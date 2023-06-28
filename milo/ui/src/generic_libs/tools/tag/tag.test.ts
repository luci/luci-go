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

import { attachTags, hasTags, TAG_SOURCE } from './tag';

const TAG_1 = {};
const TAG_2 = {};

describe('tag', () => {
  test('can attach a tag to an object', () => {
    const obj1 = {};
    const obj2 = {};
    attachTags(obj1, TAG_1);
    attachTags(obj2, TAG_2);

    expect(hasTags(obj1, TAG_1)).toBeTruthy();
    expect(hasTags(obj2, TAG_2)).toBeTruthy();

    expect(hasTags(obj1, TAG_2)).toBeFalsy();
    expect(hasTags(obj2, TAG_1)).toBeFalsy();
  });

  test('can attach multiple tags to an object', () => {
    const obj = {};

    attachTags(obj, TAG_1, TAG_2);

    expect(hasTags(obj, TAG_1, TAG_2)).toBeTruthy();
  });

  test('can detect tag attached to [TAG_SOURCE]', () => {
    const innerObj = {};
    const outerObj = {
      [TAG_SOURCE]: innerObj,
    };

    attachTags(innerObj, TAG_1);
    attachTags(outerObj, TAG_2);

    expect(hasTags(innerObj, TAG_1)).toBeTruthy();
    expect(hasTags(innerObj, TAG_2)).toBeFalsy();
    expect(hasTags(innerObj, TAG_1, TAG_2)).toBeFalsy();

    expect(hasTags(outerObj, TAG_1)).toBeTruthy();
    expect(hasTags(outerObj, TAG_2)).toBeTruthy();
    expect(hasTags(outerObj, TAG_1, TAG_2)).toBeTruthy();
  });

  test('calling attachTags multiple times should not override previous calls', () => {
    const obj = {};

    attachTags(obj, TAG_1);
    attachTags(obj, TAG_2);

    expect(hasTags(obj, TAG_1, TAG_2)).toBeTruthy();
  });
});
