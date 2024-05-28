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

import { parseInvId } from './invocation_utils';

describe('parseInvId', () => {
  describe('build Invocation ID', () => {
    it('valid', () => {
      expect(parseInvId('build-1234')).toEqual({
        type: 'build',
        buildId: '1234',
      });
    });

    it('invalid', () => {
      expect(parseInvId('build-notanumber')).toEqual({
        type: 'generic',
        invId: 'build-notanumber',
      });
    });
  });

  describe('swarming task ID', () => {
    it('valid', () => {
      expect(parseInvId('task-chromium-swarm-dev.appspot.com-a0134d')).toEqual({
        type: 'swarming-task',
        swarmingHost: 'chromium-swarm-dev.appspot.com',
        taskId: 'a0134d',
      });
    });

    it('no host', () => {
      expect(parseInvId('task-a0134d')).toEqual({
        type: 'generic',
        invId: 'task-a0134d',
      });
    });

    it('not an allowed host', () => {
      expect(parseInvId('task-chromium-swarm-d.appspot.com-a0134d')).toEqual({
        type: 'generic',
        invId: 'task-chromium-swarm-d.appspot.com-a0134d',
      });
    });

    it('no task', () => {
      expect(parseInvId('task-chromium-swarm-dev.appspot.com')).toEqual({
        type: 'generic',
        invId: 'task-chromium-swarm-dev.appspot.com',
      });
    });

    it('invalid task', () => {
      expect(
        parseInvId('task-chromium-swarm-dev.appspot.com-notatask'),
      ).toEqual({
        type: 'generic',
        invId: 'task-chromium-swarm-dev.appspot.com-notatask',
      });
    });
  });
});
