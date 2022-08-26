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

import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { TransientKeySet } from './transient_key_set';

describe('TransientKeySet', () => {
  let timer: sinon.SinonFakeTimers;

  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });

  afterEach(() => {
    timer.restore();
  });

  it('e2e', async () => {
    const keySet = TransientKeySet.create();
    after(() => destroy(keySet));
    const timestamp0 = timer.now;
    keySet.add('key0');
    expect(keySet.has('key0')).to.be.true;
    expect(keySet.has('key1')).to.be.false;
    expect(keySet.has('key2')).to.be.false;

    timer.tick(1000);
    const timestamp1 = timer.now;
    keySet.add('key1');
    keySet.add('key2');
    expect(keySet.has('key0')).to.be.true;
    expect(keySet.has('key1')).to.be.true;
    expect(keySet.has('key2')).to.be.true;

    timer.tick(1000);
    const timestamp2 = timer.now;
    keySet.add('key1'); // refresh key1

    timer.tick(1000);
    const timestamp3 = timer.now;

    // timestamp is exclusive.
    keySet.deleteStaleKeys(new Date(timestamp0));
    expect(keySet.has('key0')).to.be.true;
    expect(keySet.has('key1')).to.be.true;
    expect(keySet.has('key2')).to.be.true;

    keySet.deleteStaleKeys(new Date(timestamp1));
    expect(keySet.has('key0')).to.be.false;
    expect(keySet.has('key1')).to.be.true;
    expect(keySet.has('key2')).to.be.true;

    keySet.deleteStaleKeys(new Date(timestamp2));
    expect(keySet.has('key0')).to.be.false;
    expect(keySet.has('key1')).to.be.true; // key1 was refreshed.
    expect(keySet.has('key2')).to.be.false;

    keySet.deleteStaleKeys(new Date(timestamp3));
    expect(keySet.has('key0')).to.be.false;
    expect(keySet.has('key1')).to.be.false;
    expect(keySet.has('key2')).to.be.false;
  });
});
