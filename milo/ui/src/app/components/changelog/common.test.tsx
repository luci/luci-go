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

import {
  bumpLastReadVersion,
  getLastReadVersion,
  parseChangelog,
} from './common';

const unreleased = `\
* unreleased update 1
* unreleased update 2
`;

const latest = `\
<!-- __RELEASE__: 2 -->
 * update 4
 * update 5
`;

const past = `\
<!-- __RELEASE__: 1 -->
# 2020-02-02
 * update 2
 * update 1

<!-- __RELEASE__: 0 -->
# 2020-02-01
 * update 0
`;

const changelog = `\
${unreleased}\
${latest}\
${past}\
`;

const noUnreleasedChangelog = `\
${latest}\
${past}\
`;

const noPastChangelog = `\
${unreleased}\
${latest}\
`;

const onlyUnreleasedChangelog = `\
${unreleased}\
`;

describe('parseChangelog', () => {
  it('can parse changelog', () => {
    const log = parseChangelog(changelog);
    expect(log.latestVersion).toEqual(2);
    expect(log.latest).toEqual(latest);
    expect(log.past).toEqual(past);
  });

  it('can parse changelog without unreleased changes', () => {
    const log = parseChangelog(noUnreleasedChangelog);
    expect(log.latestVersion).toEqual(2);
    expect(log.latest).toEqual(latest);
    expect(log.past).toEqual(past);
  });

  it('can parse changelog without past changes', () => {
    const log = parseChangelog(noPastChangelog);
    expect(log.latestVersion).toEqual(2);
    expect(log.latest).toEqual(latest);
    expect(log.past).toEqual('');
  });

  it('can parse changelog with only unreleased changes', () => {
    const log = parseChangelog(onlyUnreleasedChangelog);
    expect(log.latestVersion).toEqual(-1);
    expect(log.latest).toEqual('');
    expect(log.past).toEqual('');
  });

  it('can parse empty changelog', () => {
    const log = parseChangelog('');
    expect(log.latestVersion).toEqual(-1);
    expect(log.latest).toEqual('');
    expect(log.past).toEqual('');
  });
});

describe('get/bumpLastReadVersion', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('can read from nothing', () => {
    expect(getLastReadVersion()).toEqual(-1);
  });

  it('can store last read version', () => {
    expect(getLastReadVersion()).toEqual(-1);
    bumpLastReadVersion(10);
    expect(getLastReadVersion()).toEqual(10);
  });

  it('store version in persisted storage', () => {
    expect(getLastReadVersion()).toEqual(-1);
    bumpLastReadVersion(10);
    expect(getLastReadVersion()).toEqual(10);
    localStorage.clear();
    expect(getLastReadVersion()).toEqual(-1);
  });

  it('does not decrease version', () => {
    bumpLastReadVersion(10);
    expect(getLastReadVersion()).toEqual(10);
    bumpLastReadVersion(5);
    expect(getLastReadVersion()).toEqual(10);
  });
});
