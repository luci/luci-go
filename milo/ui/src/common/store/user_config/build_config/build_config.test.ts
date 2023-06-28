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

import { destroy } from 'mobx-state-tree';

import { BuildStepsConfig, BuildStepsConfigInstance } from './build_config';

describe('BuildStepsConfig', () => {
  let store: BuildStepsConfigInstance;

  beforeEach(() => {
    store = BuildStepsConfig.create({});
  });
  afterEach(() => {
    destroy(store);
  });

  test('should set pins recursively', async () => {
    // When pinned is true, set pins for all ancestors.
    store.setStepPin('parent|step|child', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2|child1')).toBeFalsy();

    store.setStepPin('parent|step|child2', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2|child1')).toBeFalsy();

    store.setStepPin('parent|step2|child', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2|child')).toBeTruthy();

    // When pinned is false, remove pins for all descendants.
    store.setStepPin('parent|step', false);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeFalsy();
    expect(store.stepIsPinned('parent|step|child')).toBeFalsy();
    expect(store.stepIsPinned('parent|step|child2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2|child')).toBeTruthy();
  });
});
