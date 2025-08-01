// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { GridRowParams } from '@mui/x-data-grid';

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
      const task = {
        state: TaskState.COMPLETED,
        failure: false,
      } as unknown as TaskResultResponse;
      expect(prettifySwarmingState(task)).toBe('SUCCESS');
    });

    it('should return FAILURE for completed tasks with failures', () => {
      const task = {
        state: TaskState.COMPLETED,
        failure: true,
      } as unknown as TaskResultResponse;
      expect(prettifySwarmingState(task)).toBe('FAILURE');
    });

    it('should return the state as a string for non-completed tasks', () => {
      const task = {
        state: TaskState.RUNNING,
      } as unknown as TaskResultResponse;
      expect(prettifySwarmingState(task)).toBe('RUNNING');
    });
  });

  describe('getRowClassName', () => {
    it('should return row--failure for FAILURE result', () => {
      const params = { row: { result: 'FAILURE' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('row--failure');
    });

    it('should return row--pending for ongoing states', () => {
      const params = { row: { result: 'RUNNING' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('row--pending');
    });

    it('should return row--bot_died for BOT_DIED result', () => {
      const params = { row: { result: 'BOT_DIED' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('row--bot_died');
    });

    it('should return row--client_error for CLIENT_ERROR result', () => {
      const params = { row: { result: 'CLIENT_ERROR' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('row--client_error');
    });

    it('should return row--exception for exceptional states', () => {
      const params = { row: { result: 'TIMED_OUT' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('row--exception');
    });

    it('should return empty string for other states', () => {
      const params = { row: { result: 'SUCCESS' } } as GridRowParams;
      expect(getRowClassName(params)).toBe('');
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
      const task = { duration: 123 } as unknown as TaskResultResponse;
      expect(getTaskDuration(task)).toBe('2 min, 3 sec');
    });

    it('should calculate and return duration for running tasks', () => {
      const startedTs = new Date('2025-01-01T00:00:00Z');
      jest.setSystemTime(new Date('2025-01-01T00:01:05Z'));
      const task = {
        state: TaskState.RUNNING,
        startedTs: startedTs.toISOString(),
      } as unknown as TaskResultResponse;
      expect(getTaskDuration(task)).toBe('1 min, 5 sec');
    });

    it('should return 0s for tasks without duration and not running', () => {
      const task = {
        state: TaskState.PENDING,
      } as unknown as TaskResultResponse;
      expect(getTaskDuration(task)).toBe('0s');
    });
  });

  describe('getTaskTagValue', () => {
    it('should return the value of an existing tag', () => {
      const task = {
        tags: ['key1:value1', 'key2:value2'],
      } as unknown as TaskResultResponse;
      expect(getTaskTagValue(task, 'key1')).toBe('value1');
    });

    it('should return undefined for a non-existing tag', () => {
      const task = {
        tags: ['key1:value1', 'key2:value2'],
      } as unknown as TaskResultResponse;
      expect(getTaskTagValue(task, 'key3')).toBeUndefined();
    });

    it('should return undefined for a tag without a value', () => {
      const task = { tags: ['key1:'] } as unknown as TaskResultResponse;
      expect(getTaskTagValue(task, 'key1')).toBeUndefined();
    });

    it('should handle tags without a separator', () => {
      const task = { tags: ['key1'] } as unknown as TaskResultResponse;
      expect(getTaskTagValue(task, 'key1')).toBeUndefined();
    });

    it('should return the value for the first matching tag', () => {
      const task = {
        tags: ['key1:value1', 'key1:value2'],
      } as unknown as TaskResultResponse;
      expect(getTaskTagValue(task, 'key1')).toBe('value1');
    });
  });
});
