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

import { DateTime } from 'luxon';

import {
  statusToJSON,
  Status,
  statusFromJSON,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

const CREATED_BEFORE_PARAM_KEY = 'createdBefore';
const STATUS_PARAM_KEY = 'status';

export function getCreatedBefore(params: URLSearchParams) {
  const createdBefore = params.get(CREATED_BEFORE_PARAM_KEY);
  return createdBefore ? DateTime.fromSeconds(Number(createdBefore)) : null;
}

export function createdBeforeUpdater(newCreatedBefore: DateTime | null) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (!newCreatedBefore) {
      searchParams.delete(CREATED_BEFORE_PARAM_KEY);
    } else {
      searchParams.set(
        CREATED_BEFORE_PARAM_KEY,
        String(newCreatedBefore.toSeconds()),
      );
    }
    return searchParams;
  };
}

const DEFAULT_STATUS = Status.ENDED_MASK;

export function getStatus(params: URLSearchParams) {
  const status = params.get(STATUS_PARAM_KEY);
  try {
    return status ? statusFromJSON(status) : DEFAULT_STATUS;
  } catch (err) {
    return DEFAULT_STATUS;
  }
}

export function statusUpdater(newStatus: Status) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (newStatus === DEFAULT_STATUS) {
      searchParams.delete(STATUS_PARAM_KEY);
    } else {
      searchParams.set(STATUS_PARAM_KEY, statusToJSON(newStatus));
    }
    return searchParams;
  };
}
