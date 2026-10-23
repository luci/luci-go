// Copyright 2026 The LUCI Authors.
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

import { DateFilterCategoryDataBuilder } from '@/fleet/components/filters/date_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';

import { formatDeviceStateLabel } from '../chromeos/dut_state';

import { getAndroidColumnHeader } from './android_fields';
import { androidState } from './android_state';

export const ANDROID_EXTRA_FILTERS = {
  ['"fc_offline_since"']: new DateFilterCategoryDataBuilder()
    .setLabel(getAndroidColumnHeader('fc_offline_since'))
    .disableFuture(),
  ['labels."mh.last_sync"']: new DateFilterCategoryDataBuilder()
    .setLabel(getAndroidColumnHeader('mh.last_sync'))
    .disableFuture(),
  ['labels."ufs.last_sync"']: new DateFilterCategoryDataBuilder()
    .setLabel(getAndroidColumnHeader('ufs.last_sync'))
    .disableFuture(),
  ['labels."ufs.nlyte_last_sync"']: new DateFilterCategoryDataBuilder()
    .setLabel(getAndroidColumnHeader('ufs.nlyte_last_sync'))
    .disableFuture(),
  ['labels."ufs.nlyte_update_time"']: new DateFilterCategoryDataBuilder()
    .setLabel(getAndroidColumnHeader('ufs.nlyte_update_time'))
    .disableFuture(),
  ['"state"']: new StringListFilterCategoryBuilder()
    .setLabel(getAndroidColumnHeader('state'))
    .setOptions([
      { label: '(Blank)', value: '(Blank)' },
      ...Object.values(androidState).map((val) => ({
        label: formatDeviceStateLabel(val),
        value: val,
      })),
    ]),
};
