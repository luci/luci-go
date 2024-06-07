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
  DEFAULT_EXTRA_ZONE_CONFIGS,
  Timestamp,
} from '@/common/components/timestamp';
import {
  NUMERIC_TIME_FORMAT,
  SHORT_TIME_FORMAT,
} from '@/common/tools/time_utils';

export function CompactTimestamp({ datetime }: { datetime: DateTime }) {
  return (
    <Timestamp
      datetime={datetime}
      // Use a more compact format to diaply the timestamp.
      format={SHORT_TIME_FORMAT}
      extraTimezones={{
        // Use a more detailed format in the tooltip.
        format: NUMERIC_TIME_FORMAT,
        zones: [
          // Add a local timezone to display the timestamp in local timezone
          // with a more detailed format.
          {
            label: 'LOCAL',
            zone: 'local',
          },
          ...DEFAULT_EXTRA_ZONE_CONFIGS,
        ],
      }}
    />
  );
}
