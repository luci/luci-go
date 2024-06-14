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

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  Status,
  Trinary,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { OutputBuild } from '../types';

export const failedBuild = Build.fromPartial({
  id: '1234',
  status: Status.FAILURE,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  startTime: '2020-12-12T02:01:01',
  endTime: '2020-12-12T03:01:01',
}) as OutputBuild;

export const unretriableBuild = Build.fromPartial({
  id: '1234',
  status: Status.FAILURE,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  retriable: Trinary.NO,
  createTime: '2020-12-12T01:01:01',
  startTime: '2020-12-12T02:01:01',
  endTime: '2020-12-12T03:01:01',
}) as OutputBuild;

export const canaryFailedBuild = Build.fromPartial({
  id: '1234',
  status: Status.INFRA_FAILURE,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  input: {
    experiments: ['luci.buildbucket.canary_software'],
  },
}) as OutputBuild;

export const canarySucceededBuild = Build.fromPartial({
  id: '1234',
  status: Status.SUCCESS,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  input: {
    experiments: ['luci.buildbucket.canary_software'],
  },
}) as OutputBuild;

export const runningBuild = Build.fromPartial({
  id: '1234',
  status: Status.STARTED,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  startTime: '2020-12-12T02:01:01',
}) as OutputBuild;

export const scheduledToBeCanceledBuild = Build.fromPartial({
  id: '1234',
  status: Status.STARTED,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  cancelTime: '2020-12-12T02:01:01',
  canceledBy: 'user:bb_user@google.com',
  gracePeriod: { seconds: '30' },
}) as OutputBuild;

export const alreadyCanceledBuild = Build.fromPartial({
  id: '1234',
  status: Status.CANCELED,
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
  cancelTime: '2020-12-12T02:01:01',
  canceledBy: 'user:bb_user@google.com',
  gracePeriod: { seconds: '30' },
}) as OutputBuild;

export const resourceExhaustionBuild = Build.fromPartial({
  id: '1234',
  status: Status.FAILURE,
  statusDetails: {
    resourceExhaustion: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
}) as OutputBuild;

export const timeoutBuild = Build.fromPartial({
  id: '1234',
  status: Status.FAILURE,
  statusDetails: {
    timeout: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
}) as OutputBuild;

export const succeededTimeoutBuild = Build.fromPartial({
  id: '1234',
  status: Status.SUCCESS,
  statusDetails: {
    timeout: {},
  },
  builder: {
    project: 'proj',
    bucket: 'bucket',
    builder: 'builder',
  },
  createTime: '2020-12-12T01:01:01',
}) as OutputBuild;
