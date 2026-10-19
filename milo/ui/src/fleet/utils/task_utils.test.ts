// Copyright 2025 The LUCI Authors.
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
  TaskResultResponse,
  TaskState,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

import {
  getTaskDuration,
  getTaskTagValue,
  getRowClassName,
  prettifySwarmingState,
} from './task_utils';

describe('task utils', () => {
  describe('prettifySwarmingState', () => {
    it('should return SUCCESS for completed tasks without failures', () => {
      const task = TaskResultResponse.fromPartial({
        state: TaskState.COMPLETED,
        failure: false,
      });
      expect(prettifySwarmingState(task)).toBe('SUCCESS');
    });

    it('should return FAILURE for completed tasks with failures', () => {
      const task = TaskResultResponse.fromPartial({
        state: TaskState.COMPLETED,
        failure: true,
      });
      expect(prettifySwarmingState(task)).toBe('FAILURE');
    });

    it('should return the state as a string for non-completed tasks', () => {
      const task = TaskResultResponse.fromPartial({
        state: TaskState.RUNNING,
      });
      expect(prettifySwarmingState(task)).toBe('RUNNING');
    });
  });

  describe('getRowClassName', () => {
    it.each([
      ['FAILURE', 'row--failure'],
      ['RUNNING', 'row--pending'],
      ['BOT_DIED', 'row--bot_died'],
      ['CLIENT_ERROR', 'row--client_error'],
      ['TIMED_OUT', 'row--exception'],
      ['SUCCESS', ''],
    ])('should map result %s to class %s', (result, expected) => {
      expect(getRowClassName({ result })).toBe(expected);
    });
  });

  describe('getTaskDuration', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    it('should return pretty printed duration if duration is set', () => {
      const task = TaskResultResponse.fromPartial({ duration: 123 });
      expect(getTaskDuration(task)).toBe('2 min, 3 sec');
    });

    it('should calculate and return duration for running tasks', () => {
      const startedTs = new Date('2025-01-01T00:00:00Z');
      jest.setSystemTime(new Date('2025-01-01T00:01:05Z'));
      const task = TaskResultResponse.fromPartial({
        state: TaskState.RUNNING,
        startedTs: startedTs.toISOString(),
      });
      expect(getTaskDuration(task)).toBe('1 min, 5 sec');
    });

    it('should return 0s for tasks without duration and not running', () => {
      const task = TaskResultResponse.fromPartial({
        state: TaskState.PENDING,
      });
      expect(getTaskDuration(task)).toBe('0s');
    });
  });

  describe('getTaskTagValue', () => {
    it.each([
      {
        description: 'returns the value of an existing tag',
        tags: ['key1:value1', 'key2:value2'],
        tagKey: 'key1',
        expected: 'value1',
      },
      {
        description: 'returns undefined for a non-existing tag',
        tags: ['key1:value1', 'key2:value2'],
        tagKey: 'key3',
        expected: undefined,
      },
      {
        description: 'returns undefined for a tag without a value',
        tags: ['key1:'],
        tagKey: 'key1',
        expected: undefined,
      },
      {
        description: 'handles tags without a separator',
        tags: ['key1'],
        tagKey: 'key1',
        expected: undefined,
      },
      {
        description: 'returns the value for the first matching tag',
        tags: ['key1:value1', 'key1:value2'],
        tagKey: 'key1',
        expected: 'value1',
      },
    ])('should $description', ({ tags, tagKey, expected }) => {
      const task = TaskResultResponse.fromPartial({ tags });
      expect(getTaskTagValue(task, tagKey)).toBe(expected);
    });
  });
});
