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
import { assert } from 'chai';
import { autorun } from 'mobx';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { Build, GetBuildRequest } from '../services/buildbucket';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { AppState } from './app_state';
import { BuildState } from './build_state';

describe('BuildState', () => {
  let getBuildStub: sinon.SinonStub<[req: GetBuildRequest, cacheOpt?: CacheOption], Promise<Build>>;
  let appState: AppState;
  let buildState: BuildState;

  beforeEach(() => {
    const builderId = { project: 'proj', bucket: 'bucket', builder: 'builder' };
    appState = new AppState();
    appState.authState = { identity: ANONYMOUS_IDENTITY };
    getBuildStub = sinon.stub(appState.buildsService!, 'getBuild');
    getBuildStub.onCall(0).resolves({ number: 1, id: '2', builder: builderId } as Build);
    getBuildStub.onCall(1).resolves({ number: 1, id: '2', builder: builderId } as Build);

    buildState = new BuildState(appState);
    buildState.buildNumOrIdParam = '1';
    buildState.builderIdParam = builderId;
  });

  afterEach(() => {
    buildState.dispose();
    appState.dispose();
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
    appState.refresh();
    await aTimeout(0);

    assert.notStrictEqual(getBuildStub.getCall(0).args[1]?.acceptCache, false);
    assert.strictEqual(getBuildStub.getCall(1).args[1]?.acceptCache, false);
  });
});
