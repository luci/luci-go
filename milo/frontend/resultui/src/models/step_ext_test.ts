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

import { assert } from 'chai';

import { BuildStatus } from '../services/buildbucket';
import { StepExt } from './step_ext';

describe('StepExt', () => {
  function createStep(name: string, status = BuildStatus.Success) {
    const startTime = {seconds: 100, nanos: 0};
    return new StepExt({
      name,
      startTime,
      status,
    });
  }

  it('should compute parent/child names correctly', async () => {
    const step1 = createStep('child');
    assert.strictEqual(step1.parentName, null);
    assert.strictEqual(step1.selfName, 'child');

    const step2 = createStep('parent|child');
    assert.strictEqual(step2.parentName, 'parent');
    assert.strictEqual(step2.selfName, 'child');

    const step3 = createStep('grand-parent|parent|child');
    assert.strictEqual(step3.parentName, 'grand-parent|parent');
    assert.strictEqual(step3.selfName, 'child');
  });

  describe('succeededRecursively', () => {
    it('succeeded step with no children should return true', async () => {
      const step = createStep('child', BuildStatus.Success);
      assert.isTrue(step.succeededRecursively);
    });

    it('failed step with no children should return false', async () => {
      const step = createStep('child', BuildStatus.Failure);
      assert.isFalse(step.succeededRecursively);
    });

    it('failed step with succeeded children should return false', async () => {
      const step = createStep('parent', BuildStatus.Failure);
      step.children.push(createStep('parent|child1', BuildStatus.Success));
      step.children.push(createStep('parent|child2', BuildStatus.Success));
      assert.isFalse(step.succeededRecursively);
    });

    it('succeeded step with failed child should return false', async () => {
      const step = createStep('parent', BuildStatus.Success);
      step.children.push(createStep('parent|child1', BuildStatus.Success));
      step.children.push(createStep('parent|child2', BuildStatus.Failure));
      assert.isFalse(step.succeededRecursively);
    });
  });
});
