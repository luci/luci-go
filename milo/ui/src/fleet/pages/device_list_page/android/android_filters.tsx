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

import { DateFilterCategoryDataBuilder } from '@/fleet/components/filters/date_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';

import { androidState } from './android_state';

export const ANDROID_EXTRA_FILTERS = {
  fc_offline_since: new DateFilterCategoryDataBuilder().setLabel(
    `Offline Since`,
  ),
  ['"state"']: new StringListFilterCategoryBuilder()
    .setLabel('State')
    .setOptions([
      { label: '(Blank)', value: '(Blank)' },
      ...Object.values(androidState).map((val) => ({
        label: val,
        value: '"' + val + '"',
      })),
    ]),
};
