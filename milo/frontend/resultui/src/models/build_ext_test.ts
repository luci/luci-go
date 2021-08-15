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

import * as chai from 'chai';
import { assert } from 'chai';
import { DateTime } from 'luxon';
import { observable } from 'mobx';

import { chaiDeepIncludeProperties } from '../libs/test_utils/chai_deep_include_properties';
import { Build, BuildStatus, Step } from '../services/buildbucket';
import { BuildExt } from './build_ext';
import { StepExt } from './step_ext';

chai.use(chaiDeepIncludeProperties);

describe('BuildExt', () => {
  it('should build step-tree correctly', async () => {
    const time = '2020-11-01T21:43:03.351951Z';
    const build = new BuildExt({
      steps: [
        { name: 'root1', startTime: time } as Step,
        { name: 'root2', startTime: time },
        { name: 'root2|parent1', startTime: time },
        { name: 'root3', startTime: time },
        { name: 'root2|parent1|child1', startTime: time },
        { name: 'root3|parent1', startTime: time },
        { name: 'root2|parent1|child2', startTime: time },
        { name: 'root3|parent2', startTime: time },
        { name: 'root3|parent2|child1', startTime: time },
        { name: 'root3|parent2|child2', startTime: time },
      ] as readonly Step[],
    } as Build);

    assert.deepIncludeProperties(build.rootSteps, [
      {
        name: 'root1',
        selfName: 'root1',
        depth: 0,
        index: 0,
        children: [] as StepExt[],
      },
      {
        name: 'root2',
        selfName: 'root2',
        depth: 0,
        index: 1,
        children: [
          {
            name: 'root2|parent1',
            selfName: 'parent1',
            depth: 1,
            index: 0,
            children: [
              {
                name: 'root2|parent1|child1',
                selfName: 'child1',
                depth: 2,
                index: 0,
                children: [],
              },
              {
                name: 'root2|parent1|child2',
                selfName: 'child2',
                depth: 2,
                index: 1,
                children: [],
              },
            ],
          },
        ],
      },
      {
        name: 'root3',
        selfName: 'root3',
        depth: 0,
        index: 2,
        children: [
          {
            name: 'root3|parent1',
            selfName: 'parent1',
            depth: 1,
            index: 0,
            children: [],
          },
          {
            name: 'root3|parent2',
            selfName: 'parent2',
            depth: 1,
            index: 1,
            children: [
              {
                name: 'root3|parent2|child1',
                selfName: 'child1',
                depth: 2,
                index: 0,
                children: [],
              },
              {
                name: 'root3|parent2|child2',
                selfName: 'child2',
                depth: 2,
                index: 1,
                children: [],
              },
            ],
          },
        ],
      },
    ] as StepExt[]);
  });

  describe('should calculate pending/execution time/status correctly', () => {
    it("when the build hasn't started", () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Scheduled,
          createTime: '2020-01-01T00:00:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:20Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT10S');
      assert.isTrue(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration, null);
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was canceled before exceeding the scheduling timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          endTime: '2020-01-01T00:00:20Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:50Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT10S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration, null);
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was canceled after exceeding the scheduling timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          endTime: '2020-01-01T00:00:30Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:50Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT20S');
      assert.isFalse(build.isPending);
      assert.isTrue(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration, null);
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was started', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Started,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:30Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT10S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration?.toISO(), 'PT10S');
      assert.isTrue(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was started and canceled before exceeding the execution timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          endTime: '2020-01-01T00:00:30Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:40Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT10S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration?.toISO(), 'PT10S');
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build started and ended after exceeding the execution timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          endTime: '2020-01-01T00:00:40Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:50Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT10S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration?.toISO(), 'PT20S');
      assert.isFalse(build.isExecuting);
      assert.isTrue(build.exceededExecutionTimeout);
    });

    it("when the build wasn't started or canceled after the scheduling timeout", () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Scheduled,
          createTime: '2020-01-01T00:00:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:50Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT40S');
      assert.isTrue(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration, null);
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was started after the scheduling timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Started,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:40Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:00:50Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT30S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration?.toISO(), 'PT10S');
      assert.isTrue(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });

    it('when the build was not canceled after the execution timeout', () => {
      const build = new BuildExt(
        {
          status: BuildStatus.Success,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:40Z',
          endTime: '2020-01-01T00:01:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
        observable.box(DateTime.fromISO('2020-01-01T00:01:10Z'))
      );

      assert.strictEqual(build.pendingDuration.toISO(), 'PT30S');
      assert.isFalse(build.isPending);
      assert.isFalse(build.exceededSchedulingTimeout);

      assert.strictEqual(build.executionDuration?.toISO(), 'PT30S');
      assert.isFalse(build.isExecuting);
      assert.isFalse(build.exceededExecutionTimeout);
    });
  });
});
