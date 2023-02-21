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

import {
  getBuildbucketLink,
  getCipdLink,
  getLogdogRawUrl,
  getSafeUrlFromTagValue,
  renderBugUrlTemplate,
} from './build_utils';

describe('Build Utils Tests', () => {
  describe('Get Logdog URL', () => {
    it('should get correct logdog url for prod', async () => {
      const logdogURL =
        // eslint-disable-next-line max-len
        'logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket.appspot.com/8865799866429037440/+/steps/compile__with_patch_/0/stdout';
      assert.strictEqual(
        getLogdogRawUrl(logdogURL),
        // eslint-disable-next-line max-len
        'https://logs.chromium.org/logs/chromium/buildbucket/cr-buildbucket.appspot.com/8865799866429037440/+/steps/compile__with_patch_/0/stdout?format=raw'
      );
    });
    it('should get correct logdog url for dev', async () => {
      const logdogURL =
        // eslint-disable-next-line max-len
        'logdog://luci-logdog-dev.appspot.com/chromium/buildbucket/cr-buildbucket-dev.appspot.com/8865403731021509824/+/u/ensure_output_directory/stdout';
      assert.strictEqual(
        getLogdogRawUrl(logdogURL),
        // eslint-disable-next-line max-len
        'https://luci-logdog-dev.appspot.com/logs/chromium/buildbucket/cr-buildbucket-dev.appspot.com/8865403731021509824/+/u/ensure_output_directory/stdout?format=raw'
      );
    });
    it('should return null for invalid url', async () => {
      const logdogURL = 'http://notvalidurl.com';
      assert.isNull(getLogdogRawUrl(logdogURL));
    });
  });

  describe('Get Buildbucket Link', () => {
    it('should get correct Buildbucket url', async () => {
      const buildbucketLink = getBuildbucketLink('cr-buildbucket-dev.appspot.com', '123');
      assert.strictEqual(
        buildbucketLink.url,
        // eslint-disable-next-line max-len
        'https://cr-buildbucket-dev.appspot.com/rpcexplorer/services/buildbucket.v2.Builds/GetBuild?request=%7B%22id%22%3A%22123%22%7D'
      );
    });
  });

  describe('getSafeUrlFromTagValue', () => {
    it('should get the correct gerrit url', async () => {
      const url = getSafeUrlFromTagValue('patch/gerrit/chromium-review.googlesource.com/2365643/6');
      assert.strictEqual(url, 'https://chromium-review.googlesource.com/c/2365643/6');
    });

    it('should get the correct gitiles url', async () => {
      const url = getSafeUrlFromTagValue(
        'commit/gitiles/chromium.googlesource.com/chromium/src/+/6ae002709a6e0df5f61428d962e44a62920e76e1'
      );
      assert.strictEqual(
        url,
        'https://chromium.googlesource.com/chromium/src/+/6ae002709a6e0df5f61428d962e44a62920e76e1'
      );
    });

    it('should get the correct milo url', async () => {
      const url = getSafeUrlFromTagValue('build/milo/project_name/bucket/Builder name/42');
      assert.strictEqual(url, '/p/project_name/builders/bucket/Builder%20name/42');
    });
    it('should get the correct swarming url', async () => {
      const url = getSafeUrlFromTagValue('task/swarming/chrome-swarming/deadbeef');
      assert.strictEqual(url, 'https://chrome-swarming.appspot.com/task?id=deadbeef&o=true&w=true');
    });
  });

  describe('renderBugUrlTemplate', () => {
    it('should render the chromium bug URL correctly', () => {
      const url = renderBugUrlTemplate(
        // eslint-disable-next-line max-len
        'https://bugs.chromium.org/p/chromium/issues/entry?summary=summary&description=project%3A+{{{build.builder.project}}}%0Abucket%3A+{{{build.builder.bucket}}}%0Abuilder%3A+{{{build.builder.builder}}}%0Abuilder+url%3A+{{{milo_builder_url}}}%0Abuild+url%3A+{{{milo_build_url}}}',
        {
          id: '123',
          builder: {
            project: 'testproject',
            bucket: 'testbucket',
            builder: 'test builder',
          },
        },
        'https://luci-milo-dev.appspot.com'
      );
      assert.strictEqual(
        url,
        // eslint-disable-next-line max-len
        'https://bugs.chromium.org/p/chromium/issues/entry?summary=summary&description=project%3A+testproject%0Abucket%3A+testbucket%0Abuilder%3A+test%20builder%0Abuilder+url%3A+https%3A%2F%2Fluci-milo-dev.appspot.com%2Fp%2Ftestproject%2Fbuilders%2Ftestbucket%2Ftest%2520builder%0Abuild+url%3A+https%3A%2F%2Fluci-milo-dev.appspot.com%2Fb%2F123'
      );
    });

    it('should render the buganizer bug URL correctly', () => {
      const url = renderBugUrlTemplate(
        // eslint-disable-next-line max-len
        'https://b.corp.google.com/createIssue?title=title&description=project%3A+{{{build.builder.project}}}%0Abucket%3A+{{{build.builder.bucket}}}%0Abuilder%3A+{{{build.builder.builder}}}%0Abuilder_url%3A+{{{milo_builder_url}}}%0Abuild_url%3A+{{{milo_build_url}}}',
        {
          id: '123',
          builder: {
            project: 'testproject',
            bucket: 'testbucket',
            builder: 'test builder',
          },
        },
        'https://luci-milo-dev.appspot.com'
      );
      assert.strictEqual(
        url,
        // eslint-disable-next-line max-len
        'https://b.corp.google.com/createIssue?title=title&description=project%3A+testproject%0Abucket%3A+testbucket%0Abuilder%3A+test%20builder%0Abuilder_url%3A+https%3A%2F%2Fluci-milo-dev.appspot.com%2Fp%2Ftestproject%2Fbuilders%2Ftestbucket%2Ftest%2520builder%0Abuild_url%3A+https%3A%2F%2Fluci-milo-dev.appspot.com%2Fb%2F123'
      );
    });

    it("should return empty string if the domain doesn't match", () => {
      const url = renderBugUrlTemplate(
        // eslint-disable-next-line max-len
        'https://bug.chromium.org/p/chromium/issues/entry',
        {
          id: '123',
          builder: {
            project: 'test project',
            bucket: 'test bucket',
            builder: 'test builder',
          },
        },
        'https://luci-milo-dev.appspot.com'
      );
      assert.strictEqual(url, '');
    });

    it("should return empty string if the schema doesn't match", () => {
      const url = renderBugUrlTemplate(
        // eslint-disable-next-line max-len
        'http://bugs.chromium.org/p/chromium/issues/entry',
        {
          id: '123',
          builder: {
            project: 'test project',
            bucket: 'test bucket',
            builder: 'test builder',
          },
        },
        'https://luci-milo-dev.appspot.com'
      );
      assert.strictEqual(url, '');
    });
  });

  describe('Get cipd Link', () => {
    it('should get correct cipd url', async () => {
      const cipdLink = getCipdLink('infra/3pp/tools/git/mac-amd64', 'bHf2s_9KYiixd4SlHDugMeMqrngwz2QOGB_7bUcVpUoC');
      assert.strictEqual(
        cipdLink.url,
        // eslint-disable-next-line max-len
        'https://chrome-infra-packages.appspot.com/p/infra/3pp/tools/git/mac-amd64/+/bHf2s_9KYiixd4SlHDugMeMqrngwz2QOGB_7bUcVpUoC'
      );
    });
  });
});
