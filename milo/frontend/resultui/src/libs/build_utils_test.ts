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

import { assert } from 'chai';

import { getBuildbucketLink, getLogdogRawUrl } from './build_utils';


describe('Build Utils Tests', () => {
  describe('Get Logdog URL', () => {
    it('should get correct logdog url for prod', async () => {
      const logdogURL = 'logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket.appspot.com/8865799866429037440/+/steps/compile__with_patch_/0/stdout';
      assert.strictEqual(getLogdogRawUrl(logdogURL), 'https://logs.chromium.org/logs/chromium/buildbucket/cr-buildbucket.appspot.com/8865799866429037440/+/steps/compile__with_patch_/0/stdout?format=raw');
    });
    it('should get correct logdog url for dev', async () => {
      const logdogURL = 'logdog://luci-logdog-dev.appspot.com/chromium/buildbucket/cr-buildbucket-dev.appspot.com/8865403731021509824/+/u/ensure_output_directory/stdout';
      assert.strictEqual(getLogdogRawUrl(logdogURL), 'https://luci-logdog-dev.appspot.com/logs/chromium/buildbucket/cr-buildbucket-dev.appspot.com/8865403731021509824/+/u/ensure_output_directory/stdout?format=raw');
    });
    it('should return null for invalid url', async () => {
      const logdogURL = 'http://notvalidurl.com';
      assert.isNull(getLogdogRawUrl(logdogURL));
    });
  });

  describe('Get Buildbucket Link', () => {
    it('should get correct Buildbucket url', async () => {
      const buildbucketLink = getBuildbucketLink('cr-buildbucket-dev.appspot.com', '123');
      assert.strictEqual(buildbucketLink.url, 'https://cr-buildbucket-dev.appspot.com/rpcexplorer/services/buildbucket.v2.Builds/GetBuild?request=%7B%22id%22%3A%22123%22%7D');
    });
  });
});
