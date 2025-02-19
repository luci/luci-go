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

import { DateOnly } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

export const toIsoString = (dateOnly: DateOnly | undefined): string => {
  if (!dateOnly) {
    return '';
  }
  return `${dateOnly.year}-${String(dateOnly.month).padStart(2, '0')}-${String(dateOnly.day).padStart(2, '0')}`;
};
