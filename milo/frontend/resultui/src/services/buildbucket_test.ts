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
import * as sinon from 'sinon';

import { PrpcClientExt } from '../libs/prpc_client_ext';
import { Build, BuilderID, BuildsService } from './buildbucket';

describe('BuildsService', () => {
  let builder: BuilderID;
  let buildsService: BuildsService;
  let callStub: sinon.SinonStub<
    [service: string, method: string, message: object, additionalHeaders?: { [key: string]: string }],
    Promise<unknown>
  >;

  beforeEach(() => {
    builder = { project: 'proj', bucket: 'bucket', builder: 'builder' };
    const prpcClient = new PrpcClientExt({}, () => '');
    callStub = sinon.stub(prpcClient, 'call');
    callStub.onCall(0).resolves({ id: '10', number: 11, builder } as Build);
    callStub.onCall(1).resolves({ id: '20', number: 21, builder } as Build);

    buildsService = new BuildsService(prpcClient);
  });

  it('should return cached build when querying with the same build with build ID instead of build num', async () => {
    const build1 = await buildsService.getBuild({ buildNumber: 11, builder });
    const build2 = await buildsService.getBuild({ id: '10' }, { acceptCache: true });
    assert.strictEqual(build1, build2);
    assert.strictEqual(callStub.callCount, 1);
  });

  it('should return cached build when querying with the same build with build num instead of build ID', async () => {
    const build1 = await buildsService.getBuild({ id: '10' }, { acceptCache: true });
    const build2 = await buildsService.getBuild({ buildNumber: 11, builder });
    assert.strictEqual(build1, build2);
    assert.strictEqual(callStub.callCount, 1);
  });

  it('should return new build when querying different builds', async () => {
    const build1 = await buildsService.getBuild({ buildNumber: 11, builder });
    const build2 = await buildsService.getBuild({ id: '20' }, { acceptCache: true });
    assert.notStrictEqual(build1, build2);
    assert.strictEqual(build2.id, '20');
    assert.strictEqual(callStub.callCount, 2);
  });
});
