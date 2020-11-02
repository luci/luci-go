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

import { BuildStatus } from '../services/buildbucket';
import { StepExt } from '../services/build_page';
import { getLogdogRawUrl, stepSucceededRecursive } from './build_utils';


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
  describe('stepSucceededRecursive', () => {
    const createStep = (status: BuildStatus) => {
      return {
        status,
        collapsed: false,
        name: '',
        interval: { start: '', end: '', now: ''},
        startTime: {seconds: 0, nanos: 0},
      } as StepExt;
    };
    it('succeeded step with no children should return true', async () => {
      const step = createStep(BuildStatus.Success);
      assert.isTrue(stepSucceededRecursive(step));
    });
    it('failed step with no children should return false', async () => {
      const step = createStep(BuildStatus.Failure);
      assert.isFalse(stepSucceededRecursive(step));
    });
    it('succeeded step with failed child should return false', async () => {
      const step = createStep(BuildStatus.Success);
      step.children = [
        createStep(BuildStatus.Success),
        createStep(BuildStatus.Failure),
      ];
      assert.isFalse(stepSucceededRecursive(step));
    });
  });
});