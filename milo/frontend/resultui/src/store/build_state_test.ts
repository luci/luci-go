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

import * as chai from 'chai';
import { expect } from 'chai';
import chaiSubset from 'chai-subset';
import { render } from 'lit-html';
import { DateTime } from 'luxon';
import { action, computed, makeAutoObservable } from 'mobx';
import { destroy, types } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { renderMarkdown } from '../libs/markdown_utils';
import { Build, BuildStatus, Step } from '../services/buildbucket';
import { BuildState, BuildStepState, BuildStepStateInstance, clusterBuildSteps, StepExt } from './build_state';

chai.use(chaiSubset);

const StepStateTest = types.model('StepStateTest', {
  steps: types.array(BuildStepState),
  targetStep: types.optional(types.reference(BuildStepState), 0),
});

describe('BuildStepState', () => {
  function createStep(id: number, index: number, name: string, status = BuildStatus.Success, summaryMarkdown = '') {
    const nameSegs = name.split('|');
    return {
      id,
      data: {
        name,
        startTime: '2020-11-01T21:43:03.351951Z',
        status,
        summaryMarkdown,
        listNumber: '1.1',
        selfName: nameSegs.pop()!,
        depth: nameSegs.length,
        index,
      },
    };
  }

  describe('succeededRecursively/failed', () => {
    it('succeeded step with no children', async () => {
      const store = StepStateTest.create({
        steps: [createStep(0, 0, 'parent', BuildStatus.Success)],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.true;
      expect(store.targetStep.failed).to.be.false;
    });

    it('failed step with no children', async () => {
      const store = StepStateTest.create({
        steps: [createStep(0, 0, 'parent', BuildStatus.Failure)],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.true;
    });

    it('infra-failed step with no children', async () => {
      const store = StepStateTest.create({
        steps: [createStep(0, 0, 'parent', BuildStatus.InfraFailure)],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.true;
    });

    it('non-(infra-)failed step with no children', async () => {
      const store = StepStateTest.create({
        steps: [createStep(0, 0, 'parent', BuildStatus.Canceled)],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.false;
    });

    it('succeeded step with only succeeded children', async () => {
      const store = StepStateTest.create({
        steps: [
          { ...createStep(0, 0, 'parent', BuildStatus.Success), _children: [1, 2] },
          createStep(1, 0, 'parent|child1', BuildStatus.Success),
          createStep(2, 1, 'parent|child2', BuildStatus.Success),
        ],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.true;
      expect(store.targetStep.failed).to.be.false;
    });

    it('succeeded step with failed child', async () => {
      const store = StepStateTest.create({
        steps: [
          { ...createStep(0, 0, 'parent', BuildStatus.Success), _children: [1, 2] },
          createStep(1, 0, 'parent|child1', BuildStatus.Success),
          createStep(2, 1, 'parent|child2', BuildStatus.Failure),
        ],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.true;
    });

    it('succeeded step with non-succeeded child', async () => {
      const store = StepStateTest.create({
        steps: [
          { ...createStep(0, 0, 'parent', BuildStatus.Success), _children: [1, 2] },
          createStep(1, 0, 'parent|child1', BuildStatus.Success),
          createStep(2, 1, 'parent|child2', BuildStatus.Started),
        ],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.false;
    });

    it('failed step with succeeded children', async () => {
      const store = StepStateTest.create({
        steps: [
          { ...createStep(0, 0, 'parent', BuildStatus.Failure), _children: [1, 2] },
          createStep(1, 0, 'parent|child1', BuildStatus.Success),
          createStep(2, 1, 'parent|child2', BuildStatus.Success),
        ],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.true;
    });

    it('infra-failed step with succeeded children', async () => {
      const store = StepStateTest.create({
        steps: [
          { ...createStep(0, 0, 'parent', BuildStatus.InfraFailure), _children: [1, 2] },
          createStep(1, 0, 'parent|child1', BuildStatus.Success),
          createStep(2, 1, 'parent|child2', BuildStatus.Success),
        ],
      });
      after(() => destroy(store));
      expect(store.targetStep.succeededRecursively).to.be.false;
      expect(store.targetStep.failed).to.be.true;
    });
  });

  describe('summary header and content should be split properly', () => {
    function getExpectedHeaderHTML(markdownBody: string): string {
      const container = document.createElement('div');
      // Wrap in a <p> and remove it later so <!----> are not injected.
      render(renderMarkdown(`<p>${markdownBody}</p>`), container);
      return container.firstElementChild!.innerHTML;
    }

    function getExpectedBodyHTML(markdownBody: string): string {
      const container = document.createElement('div');
      render(renderMarkdown(markdownBody), container);
      return container.innerHTML;
    }

    it('for no summary', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, undefined));
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary).to.be.null;
    });

    it('for empty summary', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, ''));
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary).to.be.null;
    });

    it('for text summary', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, 'this is some text'));
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq('this is some text');
      expect(step.summary).to.be.null;
    });

    it('for header and content separated by <br/>', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, 'header<br/>content'));
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq(getExpectedHeaderHTML('header'));
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('content'));
    });

    it('for header and content separated by <br/>, header is empty', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, '<br/>body'));
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('body'));
    });

    it('for header and content separated by <br/>, body is empty', async () => {
      const step = BuildStepState.create(createStep(0, 0, 'step', BuildStatus.Success, 'header<br/>'));
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq(getExpectedHeaderHTML('header'));
      expect(step.summary).to.be.null;
    });

    it('for header and content separated by <br/>, header is a link', async () => {
      const step = BuildStepState.create(
        createStep(0, 0, 'step', BuildStatus.Success, '<a href="http://google.com">Link</a><br/>content')
      );
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq(getExpectedHeaderHTML('<a href="http://google.com">Link</a>'));
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('content'));
    });

    it('for header and content separated by <br/>, header has some inline elements', async () => {
      const step = BuildStepState.create(
        createStep(
          0,
          0,
          'step',
          BuildStatus.Success,
          '<span>span</span><i>i</i><b>b</b><strong>strong</strong><br/>content'
        )
      );
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq(
        getExpectedHeaderHTML('<span>span</span><i>i</i><b>b</b><strong>strong</strong>')
      );
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('content'));
    });

    it('for header and content separated by <br/>, header is a list', async () => {
      const step = BuildStepState.create(
        createStep(0, 0, 'step', BuildStatus.Success, '<ul><li>item</li></ul><br/>content')
      );
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('<ul><li>item</li></ul><br/>content'));
    });

    it('for header is a list', async () => {
      const step = BuildStepState.create(
        createStep(0, 0, 'step', BuildStatus.Success, '<ul><li>item1</li><li>item2</li></ul>')
      );
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('<ul><li>item1</li><li>item2</li></ul>'));
    });

    it('for <br/> is contained in <div>', async () => {
      const step = BuildStepState.create(
        createStep(0, 0, 'step', BuildStatus.Success, '<div>header<br/>other</div>content')
      );
      after(() => destroy(step));
      expect(step.header?.innerHTML).to.be.eq(getExpectedHeaderHTML('header'));
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('<div>other</div>content'));
    });

    it('for <br/> is contained in some nested tags', async () => {
      const step = BuildStepState.create(
        createStep(0, 0, 'step', BuildStatus.Success, '<div><div>header<br/>other</div></div>content')
      );
      after(() => destroy(step));
      expect(step.header).to.be.null;
      expect(step.summary?.innerHTML).to.be.eq(getExpectedBodyHTML('<div><div>header<br/>other</div></div>content'));
    });
  });
});

describe('clusterBuildSteps', () => {
  function createStep(id: number, isCritical: boolean) {
    return {
      id,
      isCritical,
    } as Partial<BuildStepStateInstance> as BuildStepStateInstance;
  }

  it('should cluster build steps correctly', () => {
    const clusteredSteps = clusterBuildSteps([
      createStep(1, false),
      createStep(2, false),
      createStep(3, false),
      createStep(4, true),
      createStep(5, false),
      createStep(6, false),
      createStep(7, true),
      createStep(8, true),
      createStep(9, false),
      createStep(10, true),
      createStep(11, true),
    ]);
    expect(clusteredSteps).to.deep.eq([
      [createStep(1, false), createStep(2, false), createStep(3, false)],
      [createStep(4, true)],
      [createStep(5, false), createStep(6, false)],
      [createStep(7, true), createStep(8, true)],
      [createStep(9, false)],
      [createStep(10, true), createStep(11, true)],
    ]);
  });

  it("should cluster build steps correctly when there're no steps", () => {
    const clusteredSteps = clusterBuildSteps([]);
    expect(clusteredSteps).to.deep.eq([]);
  });

  it("should cluster build steps correctly when there's a single step", () => {
    const clusteredSteps = clusterBuildSteps([createStep(1, false)]);
    expect(clusteredSteps).to.deep.eq([[createStep(1, false)]]);
  });

  it('should not re-cluster steps when the criticality is updated', () => {
    const step1 = makeAutoObservable(createStep(1, false));
    const step2 = makeAutoObservable(createStep(2, false));
    const step3 = makeAutoObservable(createStep(3, false));

    const computedCluster = computed(() => clusterBuildSteps([step1, step2, step3]), { keepAlive: true });

    const clustersBeforeUpdate = clusterBuildSteps([step1, step2, step3]);
    expect(clustersBeforeUpdate).to.deep.eq([[step1, step2, step3]]);
    expect(computedCluster.get()).to.deep.eq(clustersBeforeUpdate);

    action(() => ((step2 as Mutable<typeof step2>).isCritical = true))();
    const clustersAfterUpdate = clusterBuildSteps([step1, step2, step3]);
    expect(clustersAfterUpdate).to.deep.eq([[step1], [step2], [step3]]);

    expect(computedCluster.get()).to.deep.eq(clustersBeforeUpdate);
  });
});

describe('BuildState', () => {
  it('should build step-tree correctly', async () => {
    const time = '2020-11-01T21:43:03.351951Z';
    const build = BuildState.create({
      data: {
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
      } as Build,
    });
    after(() => destroy(build));

    expect(build.rootSteps).containSubset([
      {
        data: {
          name: 'root1',
          selfName: 'root1',
          listNumber: '1.',
          depth: 0,
          index: 0,
        } as Partial<StepExt>,
        children: [] as BuildStepStateInstance[],
      } as Partial<BuildStepStateInstance>,
      {
        data: {
          name: 'root2',
          selfName: 'root2',
          listNumber: '2.',
          depth: 0,
          index: 1,
        },
        children: [
          {
            data: {
              name: 'root2|parent1',
              selfName: 'parent1',
              listNumber: '2.1.',
              depth: 1,
              index: 0,
            },
            children: [
              {
                data: {
                  name: 'root2|parent1|child1',
                  selfName: 'child1',
                  listNumber: '2.1.1.',
                  depth: 2,
                  index: 0,
                },
                children: [],
              },
              {
                data: {
                  name: 'root2|parent1|child2',
                  selfName: 'child2',
                  listNumber: '2.1.2.',
                  depth: 2,
                  index: 1,
                },
                children: [],
              },
            ],
          },
        ],
      },
      {
        data: {
          name: 'root3',
          selfName: 'root3',
          listNumber: '3.',
          depth: 0,
          index: 2,
        },
        children: [
          {
            data: {
              name: 'root3|parent1',
              selfName: 'parent1',
              listNumber: '3.1.',
              depth: 1,
              index: 0,
            },
            children: [],
          },
          {
            data: {
              name: 'root3|parent2',
              selfName: 'parent2',
              listNumber: '3.2.',
              depth: 1,
              index: 1,
            },
            children: [
              {
                data: {
                  name: 'root3|parent2|child1',
                  selfName: 'child1',
                  listNumber: '3.2.1.',
                  depth: 2,
                  index: 0,
                },
                children: [],
              },
              {
                data: {
                  name: 'root3|parent2|child2',
                  selfName: 'child2',
                  listNumber: '3.2.2.',
                  depth: 2,
                  index: 1,
                },
                children: [],
              },
            ],
          },
        ],
      },
    ] as BuildStepStateInstance[]);
  });

  describe('should calculate pending/execution time/status correctly', () => {
    let timer: sinon.SinonFakeTimers;
    before(() => (timer = sinon.useFakeTimers()));
    after(() => timer.restore());

    it("when the build hasn't started", () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:20Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Scheduled,
          createTime: '2020-01-01T00:00:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT10S');
      expect(build.isPending).to.be.true;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration).to.be.null;
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was canceled before exceeding the scheduling timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          endTime: '2020-01-01T00:00:20Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT10S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration).to.be.null;
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was canceled after exceeding the scheduling timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          endTime: '2020-01-01T00:00:30Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT20S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.true;

      expect(build.executionDuration).to.be.null;
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was started', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:30Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Started,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT10S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration?.toISO()).to.be.eq('PT10S');
      expect(build.isExecuting).to.be.true;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was started and canceled before exceeding the execution timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:40Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          endTime: '2020-01-01T00:00:30Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT10S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration?.toISO()).to.be.eq('PT10S');
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build started and ended after exceeding the execution timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Canceled,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:20Z',
          endTime: '2020-01-01T00:00:40Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT10S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration?.toISO()).to.be.eq('PT20S');
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.true;
    });

    it("when the build wasn't started or canceled after the scheduling timeout", () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Scheduled,
          createTime: '2020-01-01T00:00:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT40S');
      expect(build.isPending).to.be.true;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration).to.be.null;
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was started after the scheduling timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Started,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:40Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT30S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration?.toISO()).to.be.eq('PT10S');
      expect(build.isExecuting).to.be.true;
      expect(build.exceededExecutionTimeout).to.be.false;
    });

    it('when the build was not canceled after the execution timeout', () => {
      timer.setSystemTime(DateTime.fromISO('2020-01-01T00:01:10Z').toMillis());
      const build = BuildState.create({
        data: {
          status: BuildStatus.Success,
          createTime: '2020-01-01T00:00:10Z',
          startTime: '2020-01-01T00:00:40Z',
          endTime: '2020-01-01T00:01:10Z',
          schedulingTimeout: '20s',
          executionTimeout: '20s',
        } as Build,
      });
      after(() => destroy(build));

      expect(build.pendingDuration.toISO()).to.be.eq('PT30S');
      expect(build.isPending).to.be.false;
      expect(build.exceededSchedulingTimeout).to.be.false;

      expect(build.executionDuration?.toISO()).to.be.eq('PT30S');
      expect(build.isExecuting).to.be.false;
      expect(build.exceededExecutionTimeout).to.be.false;
    });
  });
});
