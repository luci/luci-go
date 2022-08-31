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

import { expect } from 'chai';
import { autorun } from 'mobx';
import { addDisposer, destroy } from 'mobx-state-tree';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { Build, GetBuildRequest } from '../services/buildbucket';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { Store, StoreInstance } from '.';

describe('BuildPageStore', () => {
  describe('cache', () => {
    let timer: sinon.SinonFakeTimers;
    let store: StoreInstance;
    let getBuildStub: sinon.SinonStub<[req: GetBuildRequest, cacheOpt?: CacheOption], Promise<Build>>;

    beforeEach(() => {
      const builderId = { project: 'proj', bucket: 'bucket', builder: 'builder' };
      timer = sinon.useFakeTimers();
      store = Store.create({
        authState: { value: { identity: ANONYMOUS_IDENTITY } },
        refreshTime: { value: timer.now },
      });
      store.buildPage.setParams(builderId, '1');

      getBuildStub = sinon.stub(store.services.builds!, 'getBuild');
      getBuildStub.onCall(0).resolves({ number: 1, id: '2', builder: builderId } as Build);
      getBuildStub.onCall(1).resolves({ number: 1, id: '2', builder: builderId } as Build);
    });

    afterEach(() => {
      destroy(store);
      timer.restore();
    });

    it('should accept cache when first querying build', async () => {
      store.buildPage.build;
      await timer.runAllAsync();
      expect(getBuildStub.getCall(0).args[1]?.acceptCache).to.not.be.false;
    });

    it('should not accept cache after calling refresh', async () => {
      store.buildPage.build;
      await timer.runAllAsync();

      await timer.tickAsync(10);
      store.refreshTime.refresh();
      store.buildPage.build;
      await timer.runAllAsync();

      expect(getBuildStub.getCall(0).args[1]?.acceptCache).to.not.be.false;
      expect(getBuildStub.getCall(1).args[1]?.acceptCache).to.be.false;
    });
  });

  describe('params', () => {
    let timer: sinon.SinonFakeTimers;

    beforeEach(() => {
      timer = sinon.useFakeTimers();
    });

    afterEach(() => {
      timer.restore();
    });

    it('ignore builderIdParam when buildNumOrIdParam is a buildId', async () => {
      const store = Store.create({
        authState: { value: { identity: ANONYMOUS_IDENTITY } },
        refreshTime: { value: timer.now },
      });
      after(() => destroy(store));
      store.buildPage.setParams({ project: 'wrong_proj', bucket: 'wrong_bucket', builder: 'wrong_builder' }, 'b123');

      const getBuildStub = sinon.stub(store.services.builds!, 'getBuild');
      const getBuilderStub = sinon.stub(store.services.builders!, 'getBuilder');
      const getProjectCfgStub = sinon.stub(store.services.milo!, 'getProjectCfg');
      const batchCheckPermissionsStub = sinon.stub(store.services.milo!, 'batchCheckPermissions');

      const builderId = { project: 'proj', bucket: 'bucket', builder: 'builder' };
      getBuildStub.onCall(0).resolves({ number: 1, id: '123', builder: builderId } as Build);
      getBuilderStub.onCall(0).resolves({ id: builderId, config: {} });
      getProjectCfgStub.onCall(0).resolves({});
      batchCheckPermissionsStub.onCall(0).resolves({ results: {} });

      addDisposer(
        store,
        autorun(() => {
          store.buildPage.build;
          store.buildPage.builder;
          store.buildPage.customBugLink;
          store.buildPage.permittedActions;
        })
      );
      await timer.runAllAsync();

      expect(getBuildStub.getCall(0).args[0].builder).to.be.undefined;
      expect(getBuilderStub.getCall(0).args[0].id).to.deep.equal(builderId);
      expect(getProjectCfgStub.getCall(0).args[0].project).to.equal(builderId.project);
      expect(batchCheckPermissionsStub.getCall(0).args[0].realm).to.equal(`${builderId.project}:${builderId.bucket}`);
    });
  });
});
