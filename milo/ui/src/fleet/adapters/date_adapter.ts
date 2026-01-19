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

import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { DateTime, Settings } from 'luxon';

/**
 * A wrapper around AdapterLuxon that handles Settings.throwOnInvalid = true.
 *
 * When throwOnInvalid is true, luxon throws errors when attempting to create invalid dates.
 * However, MUI DatePicker expects to receive invalid date objects (isValid=false) instead of
 * catching exceptions during parsing or invalid date generation.
 *
 * This adapter temporarily disables throwOnInvalid when creating invalid dates to ensure
 * MUI DatePicker can handle invalid inputs gracefully without crashing the app.
 */
export class SafeAdapterLuxon extends AdapterLuxon {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  constructor(props?: any) {
    super(props);

    // Override parse after super() which sets the original parse.
    // We capture the original method to reuse its logic (formatting, locale handling, etc).
    const originalParse = this.parse;
    this.parse = (value, format) => {
      try {
        return originalParse(value, format);
      } catch (e) {
        // If luxon threw an error (likely due to throwOnInvalid=true), return an invalid date.
        return this.createInvalidDate(
          e instanceof Error ? e.message : String(e),
        );
      }
    };

    // Override getInvalidDate to prevent throwing when creating the invalid date instance.
    this.getInvalidDate = () => {
      return this.createInvalidDate('Invalid Date');
    };
  }

  // Helper to create invalid date without throwing
  private createInvalidDate(reason: string): DateTime<false> {
    const originalThrow = Settings.throwOnInvalid;
    // We only want to suppress the throw for the creation of this specific invalid date object.
    if (originalThrow) {
      Settings.throwOnInvalid = false;
    }
    try {
      return DateTime.invalid(reason);
    } finally {
      if (originalThrow) {
        Settings.throwOnInvalid = originalThrow;
      }
    }
  }
}
