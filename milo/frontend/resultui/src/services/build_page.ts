// Copyright 2020 The LUCI Authors.
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


// TODO(crbug/1108200): Replace this file with direct queries to services.

import jsonbigint from 'json-bigint';
import camelCase from 'lodash-es/camelCase';
import isArray from 'lodash-es/isArray';
import isObject from 'lodash-es/isObject';
import reduce from 'lodash-es/reduce';

import { Build } from './buildbucket';

/**
 * Converts the keys in an object to camelCase recursively.
 */
function camelCaseRecursive<T>(obj: T): T {
  if (!isObject(obj)) {
    return obj as T;
  } else if (isArray(obj)) {
    return obj.map((v) => camelCaseRecursive(v)) as unknown as T;
  }
  return reduce(obj, (r, v, k) => {
    return {
      ...r,
      [camelCase(k)]: camelCaseRecursive(v),
    };
  }, {}) as T;
}

export interface Link {
  label: string;
  url: string;
  ariaLabel?: string;
  img?: string;
  alt?: string;
  alias?: boolean;
}

export interface RelatedBuildsData {
  build: Build;
  relatedBuilds: Build[];
}

/**
 * A helper class for querying Milo build page data.
 */
export class BuildPageService {
  constructor(private accessToken: string) {}
  // tslint:disable-next-line: ban-types
  private reviver = (k: string, v: unknown) => k === 'id'?  (v as Object).toString() : v;

  async getRelatedBuilds(bbId: string): Promise<RelatedBuildsData> {
    const fetchUrl = `/related_builds/${bbId}`;
    const res = await fetch(
      fetchUrl,
      {headers: {'Authorization': `Bearer ${this.accessToken}`}},
    );
    const relatedBuildsDataText = await res.text();
    const relatedBuildsData = camelCaseRecursive(jsonbigint.parse(relatedBuildsDataText, this.reviver));
    return relatedBuildsData as RelatedBuildsData;
  }
}
