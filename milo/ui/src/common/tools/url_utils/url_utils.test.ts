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
  getOldBuilderURLPath,
  getSwarmingBotListURL,
  getTestHistoryURLPath,
  setSingleQueryParam,
} from './url_utils';

describe('getBuilderURLPath', () => {
  test('should encode the builder', () => {
    const url = getOldBuilderURLPath({
      project: 'testproject',
      bucket: 'testbucket',
      builder: 'test builder',
    });
    expect(url).toStrictEqual(
      '/p/testproject/builders/testbucket/test%20builder',
    );
  });
});

describe('getTestHisotryURLPath', () => {
  test('should encode the test ID', () => {
    const url = getTestHistoryURLPath('testproject', 'test/id');
    expect(url).toStrictEqual('/ui/test/testproject/test%2Fid');
  });
});

describe('getSwarmingBotListURL', () => {
  test('should support multiple dimensions', () => {
    const url = getSwarmingBotListURL('chromium-swarm-dev.appspot.com', [
      'cpu:x86-64',
      'os:Windows-11',
    ]);
    expect(url).toStrictEqual(
      'https://chromium-swarm-dev.appspot.com/botlist?f=cpu%3Ax86-64&f=os%3AWindows-11',
    );
  });
});

describe('setSingleQueryParam', () => {
  it('should set parameter string value', () => {
    const parameterString = 'a=b&c=d';
    const updatedParams = setSingleQueryParam(parameterString, 'e', 'f');
    expect(updatedParams.toString()).toEqual('a=b&c=d&e=f');
  });

  it('should update parameter string value', () => {
    const parameterString = 'a=b&c=d';
    const updatedParams = setSingleQueryParam(parameterString, 'a', 'f');
    expect(updatedParams.toString()).toEqual('a=f&c=d');
  });
});
