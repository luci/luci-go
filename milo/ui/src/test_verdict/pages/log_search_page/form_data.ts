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

import {
  ArtifactContentMatcher,
  IDMatcher,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
export interface FormData {
  testIDStr: string;
  isTestIDStrPrefix: boolean;
  artifactIDStr: string;
  isArtifactIDStrPrefix: boolean;
  searchStr: string;
  isSearchStrRegex: boolean;
  startTime: DateTime | null;
  endTime: DateTime | null;
}

export const FormData = {
  getSearchString(form: FormData): ArtifactContentMatcher | undefined {
    return form.searchStr === ''
      ? undefined
      : form.isSearchStrRegex
        ? { regexContain: form.searchStr }
        : { exactContain: form.searchStr };
  },

  getTestIDMatcher(form: FormData): IDMatcher | undefined {
    return form.testIDStr === ''
      ? undefined
      : form.isTestIDStrPrefix
        ? { hasPrefix: form.testIDStr }
        : { exactEqual: form.testIDStr };
  },

  getArtifactIDMatcher(form: FormData): IDMatcher | undefined {
    return form.artifactIDStr === ''
      ? undefined
      : form.isArtifactIDStrPrefix
        ? { hasPrefix: form.artifactIDStr }
        : { exactEqual: form.artifactIDStr };
  },
};
