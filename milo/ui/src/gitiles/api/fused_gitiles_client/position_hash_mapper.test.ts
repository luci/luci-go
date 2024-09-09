// Copyright 2024 The LUCI Authors.
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

import { deferred } from '@/generic_libs/tools/utils';

import { PositionHashMapper } from './position_hash_mapper';

describe('PositionHashMapper', () => {
  it('can resolve smaller commit position without sending queries', async () => {
    const getHash = jest.fn(async (pos: number) => `position-${pos}-hash`);
    const mapper = new PositionHashMapper(getHash);

    expect(await mapper.getCommitish(50)).toEqual('position-50-hash~0');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(40)).toEqual('position-50-hash~10');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(30)).toEqual('position-50-hash~20');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(20)).toEqual('position-50-hash~30');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(10)).toEqual('position-50-hash~40');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(5)).toEqual('position-50-hash~45');
    expect(getHash).toHaveBeenCalledTimes(1);
  });

  it('can resolve larger commit position by sending queries', async () => {
    const getHash = jest.fn(async (pos: number) => `position-${pos}-hash`);
    const mapper = new PositionHashMapper(getHash);

    expect(await mapper.getCommitish(50)).toEqual('position-50-hash~0');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(40)).toEqual('position-50-hash~10');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(60)).toEqual('position-60-hash~0');
    expect(getHash).toHaveBeenCalledTimes(2);
    expect(await mapper.getCommitish(55)).toEqual('position-60-hash~5');
    expect(getHash).toHaveBeenCalledTimes(2);
    expect(await mapper.getCommitish(70)).toEqual('position-70-hash~0');
    expect(getHash).toHaveBeenCalledTimes(3);
    expect(await mapper.getCommitish(65)).toEqual('position-70-hash~5');
    expect(getHash).toHaveBeenCalledTimes(3);
  });

  it('can resolve smaller commit position after resolving larger commit position', async () => {
    const getHash = jest.fn(async (pos: number) => `position-${pos}-hash`);
    const mapper = new PositionHashMapper(getHash);

    expect(await mapper.getCommitish(50)).toEqual('position-50-hash~0');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(40)).toEqual('position-50-hash~10');
    expect(getHash).toHaveBeenCalledTimes(1);
    expect(await mapper.getCommitish(60)).toEqual('position-60-hash~0');
    expect(getHash).toHaveBeenCalledTimes(2);
    expect(await mapper.getCommitish(40)).toEqual('position-60-hash~20');
    expect(getHash).toHaveBeenCalledTimes(2);
  });

  it('can avoid sending unnecessary queries even when the previous one has not resolved', async () => {
    const [block, resolveBlock] = deferred();
    const getHash = jest.fn(async (pos: number) => {
      await block;
      return `position-${pos}-hash`;
    });
    const mapper = new PositionHashMapper(getHash);

    const pos60 = mapper.getCommitish(60);
    const pos50 = mapper.getCommitish(50);
    resolveBlock();

    expect(await pos50).toEqual('position-60-hash~10');
    expect(await pos60).toEqual('position-60-hash~0');
    expect(getHash).toHaveBeenCalledTimes(1);
  });

  it('failed query does not block future queries', async () => {
    const getHash = jest.fn(async (pos: number) => `position-${pos}-hash`);
    const mapper = new PositionHashMapper(getHash);

    expect(await mapper.getCommitish(50)).toEqual('position-50-hash~0');
    expect(getHash).toHaveBeenCalledTimes(1);

    getHash.mockRejectedValueOnce('error');
    await expect(mapper.getCommitish(60)).rejects.toEqual('error');
    expect(await mapper.getCommitish(60)).toEqual('position-60-hash~0');
  });
});
