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

import { DateTime } from 'luxon';

import { OutputBuild } from '@/build/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { getTimingInfo } from './build_utils';

describe('getTimingInfo', () => {
  beforeAll(() => {
    jest.useFakeTimers();
  });
  afterAll(() => {
    jest.useRealTimers();
  });

  it("when the build hasn't started", () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:20Z').toMillis());
    const build = Build.fromPartial({
      status: Status.SCHEDULED,
      createTime: '2020-01-01T00:00:10Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT10S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration).toBeNull();
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was canceled before exceeding the scheduling timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
    const build = Build.fromPartial({
      status: Status.CANCELED,
      createTime: '2020-01-01T00:00:10Z',
      endTime: '2020-01-01T00:00:20Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT10S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration).toBeNull();
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was canceled after exceeding the scheduling timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
    const build = Build.fromPartial({
      status: Status.CANCELED,
      createTime: '2020-01-01T00:00:10Z',
      endTime: '2020-01-01T00:00:30Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT20S');
    expect(info.exceededSchedulingTimeout).toBeTruthy();

    expect(info.executionDuration).toBeNull();
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was started', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:30Z').toMillis());
    const build = Build.fromPartial({
      status: Status.STARTED,
      createTime: '2020-01-01T00:00:10Z',
      startTime: '2020-01-01T00:00:20Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT10S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration?.toISO()).toStrictEqual('PT10S');
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was started and canceled before exceeding the execution timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:40Z').toMillis());
    const build = Build.fromPartial({
      status: Status.STARTED,
      createTime: '2020-01-01T00:00:10Z',
      startTime: '2020-01-01T00:00:20Z',
      endTime: '2020-01-01T00:00:30Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT10S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration?.toISO()).toStrictEqual('PT10S');
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build started and ended after exceeding the execution timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
    const build = Build.fromPartial({
      status: Status.CANCELED,
      createTime: '2020-01-01T00:00:10Z',
      startTime: '2020-01-01T00:00:20Z',
      endTime: '2020-01-01T00:00:40Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT10S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration?.toISO()).toStrictEqual('PT20S');
    expect(info.exceededExecutionTimeout).toBeTruthy();
  });

  it("when the build wasn't started or canceled after the scheduling timeout", () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
    const build = Build.fromPartial({
      status: Status.SCHEDULED,
      createTime: '2020-01-01T00:00:10Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT40S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration).toBeNull();
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was started after the scheduling timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:00:50Z').toMillis());
    const build = Build.fromPartial({
      status: Status.STARTED,
      createTime: '2020-01-01T00:00:10Z',
      startTime: '2020-01-01T00:00:40Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT30S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration?.toISO()).toStrictEqual('PT10S');
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });

  it('when the build was not canceled after the execution timeout', () => {
    jest.setSystemTime(DateTime.fromISO('2020-01-01T00:01:10Z').toMillis());
    const build = Build.fromPartial({
      status: Status.SUCCESS,
      createTime: '2020-01-01T00:00:10Z',
      startTime: '2020-01-01T00:00:40Z',
      endTime: '2020-01-01T00:01:10Z',
      schedulingTimeout: { seconds: '20' },
      executionTimeout: { seconds: '20' },
    }) as OutputBuild;

    const info = getTimingInfo(build);

    expect(info.pendingDuration.toISO()).toStrictEqual('PT30S');
    expect(info.exceededSchedulingTimeout).toBeFalsy();

    expect(info.executionDuration?.toISO()).toStrictEqual('PT30S');
    expect(info.exceededExecutionTimeout).toBeFalsy();
  });
});
