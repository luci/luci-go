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

import { configuredTrees } from '@/monitoringv2/util/config';
import { AlertJson } from '@/monitoringv2/util/server_json';

import { fileBugLink } from './file_bug_link';

const alert: AlertJson = {
  key: 'key',
  title: 'title',
  body: 'alert body',
  severity: 0,
  time: 0,
  start_time: 0,
  links: null,
  tags: null,
  type: '',
  extension: {
    builders: [],
    culprits: null,
    has_findings: false,
    is_finished: false,
    is_supported: false,
    reason: {
      num_failing_tests: 0,
      step: '',
      tests: [],
    },
    regression_ranges: [],
    suspected_cls: null,
    tree_closer: false,
  },
  resolved: false,
  bug: '0',
  silenceUntil: '0',
};

describe('fileBugLink', () => {
  it('contains alert title', () => {
    expect(fileBugLink(configuredTrees[0], [alert])).toContain('title');
  });
  it('contains LUCI Monitoring label', () => {
    expect(fileBugLink(configuredTrees[0], [alert])).toContain(
      'Filed-Via-LUCI-Monitoring',
    );
  });
});
