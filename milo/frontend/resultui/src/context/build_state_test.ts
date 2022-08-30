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

import { aTimeout } from '@open-wc/testing/index-no-side-effects';
import { assert, expect } from 'chai';
import { autorun } from 'mobx';
import { destroy } from 'mobx-state-tree';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { Build, GetBuildRequest } from '../services/buildbucket';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { Store, StoreInstance } from '../store';
import { BuildState } from './build_state';

describe('BuildState', () => {
  describe('cache', () => {
    let getBuildStub: sinon.SinonStub<[req: GetBuildRequest, cacheOpt?: CacheOption], Promise<Build>>;
    let store: StoreInstance;
    let buildState: BuildState;

    beforeEach(() => {
      const builderId = { project: 'proj', bucket: 'bucket', builder: 'builder' };
      store = Store.create({ authState: { value: { identity: ANONYMOUS_IDENTITY } } });
      getBuildStub = sinon.stub(store.services.builds!, 'getBuild');
      getBuildStub.onCall(0).resolves({ number: 1, id: '2', builder: builderId } as Build);
      getBuildStub.onCall(1).resolves({ number: 1, id: '2', builder: builderId } as Build);

      buildState = new BuildState(store);
      buildState.buildNumOrIdParam = '1';
      buildState.builderIdParam = builderId;
    });

    afterEach(() => {
      buildState.dispose();
      destroy(store);
    });

    it('should accept cache when first querying build', async () => {
      const disposer = autorun(() => buildState.build);
      after(disposer);

      await aTimeout(0);

      assert.notStrictEqual(getBuildStub.getCall(0).args[1]?.acceptCache, false);
    });

    it('should not accept cache after calling refresh', async () => {
      const disposer = autorun(() => {
        buildState.build;
      });
      after(disposer);
      await aTimeout(0);
      store.refresh();
      await aTimeout(0);

      assert.notStrictEqual(getBuildStub.getCall(0).args[1]?.acceptCache, false);
      assert.strictEqual(getBuildStub.getCall(1).args[1]?.acceptCache, false);
    });
  });

  it('ignore builderIdParam when buildNumOrIdParam is a buildId', async () => {
    const builderId = { project: 'proj', bucket: 'bucket', builder: 'builder' };
    const store = Store.create({ authState: { value: { identity: ANONYMOUS_IDENTITY } } });

    const getBuildStub = sinon.stub(store.services.builds!, 'getBuild');
    const getBuilderStub = sinon.stub(store.services.builders!, 'getBuilder');
    const getProjectCfgStub = sinon.stub(store.services.milo!, 'getProjectCfg');
    const batchCheckPermissionsStub = sinon.stub(store.services.milo!, 'batchCheckPermissions');
    getBuildStub.onCall(0).resolves({ number: 1, id: '123', builder: builderId } as Build);
    getBuilderStub.onCall(0).resolves({ id: builderId, config: {} });
    getProjectCfgStub.onCall(0).resolves({});
    batchCheckPermissionsStub.onCall(0).resolves({ results: {} });

    const buildState = new BuildState(store);
    buildState.buildNumOrIdParam = 'b123';
    buildState.builderIdParam = { project: 'wrong_proj', bucket: 'wrong_bucket', builder: 'wrong_builder' };

    after(() => {
      buildState.dispose();
      destroy(store);
    });

    const disposer = autorun(() => {
      buildState.build;
      buildState.builder;
      buildState.customBugLink;
      buildState.permittedActions;
    });
    after(disposer);

    await aTimeout(0);

    expect(getBuildStub.getCall(0).args[0].builder).to.be.undefined;
    expect(getBuilderStub.getCall(0).args[0].id).to.deep.equal(builderId);
    expect(getProjectCfgStub.getCall(0).args[0].project).to.equal(builderId.project);
    expect(batchCheckPermissionsStub.getCall(0).args[0].realm).to.equal(`${builderId.project}:${builderId.bucket}`);
  });
});
