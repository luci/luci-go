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

import {
  ArtifactContentMatcher,
  IDMatcher,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

const FILTER_KEY = 'filter';

/**
 * FormData represent filter fields directly managed by the <LogSearch />.
 */
export interface FormData {
  testIDStr: string;
  isTestIDStrPrefix: boolean;
  artifactIDStr: string;
  isArtifactIDStrPrefix: boolean;
  searchStr: string;
  isSearchStrRegex: boolean;
}

/**
 * Empty form is the default state of the form before any user modification.
 */
export const EMPTY_FORM: FormData = {
  testIDStr: '',
  isTestIDStrPrefix: true,
  artifactIDStr: '',
  isArtifactIDStrPrefix: false,
  searchStr: '',
  isSearchStrRegex: false,
};

export const FormData = {
  fromSearchParam(searchParams: URLSearchParams): FormData | null {
    const form = searchParams.get(FILTER_KEY);
    return form ? { ...EMPTY_FORM, ...JSON.parse(form) } : null;
  },
  toSearchParamUpdater(newFormData: FormData) {
    return (params: URLSearchParams) => {
      const searchParams = new URLSearchParams(params);
      // TODO (beining@): Using JSON.stringify for the search parameter is backward incompatible
      // if any property is renamed. A better way is to have a constant search parameter key for each
      // of these field.
      searchParams.set('filter', JSON.stringify(newFormData));
      return searchParams;
    };
  },
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
