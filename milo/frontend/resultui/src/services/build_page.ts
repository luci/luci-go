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

import { Build, BuilderID, Step } from './buildbucket';

export interface GetBuildPageDataRequest {
  builder: BuilderID;
  buildNumOrId: string;
}

/**
 * BuildPageData is the type definition of the JSON returned from
 * /p/${project}/builders/${bucket}/${builder}/${build_id_or_number}/data
 *
 * Fields that are not intended to be used in the frontend are excluded.
 */
export interface BuildPageData extends Build {
  now: string;
  build_bug_link: string;
  buildbucket_host: string;
  can_cancel: boolean;
  can_retry: boolean;

  commit_link_html: string;
  summary?: string[];
  recipe_link: Link;
  buildbucket_link: Link;
  build_sets: string[];
  buildset_links: string[];
  steps?: StepExt[];
  human_status: string;
  should_show_canary_warming: boolean;
  input_properties: Property[];
  output_properties: Property[];
  builder_link: Link;
  link: Link;
  banners: Logo[];
  timeline: string;
}

export interface Property {
  name: string;
  value: string;
}

export interface StepExt extends Step {
  children?: StepExt[];
  collapsed: boolean;
  interval: Interval;
}

export interface Interval {
  start: string;
  end: string;
  now: string;
}

export interface Link {
  label: string;
  url: string;
  aria_label?: string;
  img?: string;
  alt?: string;
  alias?: boolean;
}

export interface LogoBase {
  img: string;
  alt: string;
}

export interface Logo extends LogoBase {
  subtitle: string;
  count: number;
}

export interface RelatedBuildsData {
  build: Build;
  related_builds: Build[];
}

/**
 * A helper class for querying Milo build page data.
 */
export class BuildPageService {
  constructor(private accessToken: string) {}
  // tslint:disable-next-line: ban-types
  private reviver = (k: string, v: unknown) => k === 'id'?  (v as Object).toString() : v;

  async getBuildPageData(req: GetBuildPageDataRequest): Promise<BuildPageData> {
    const res = await fetch(
      `/p/${req.builder.project}/builders/${req.builder.bucket}/${req.builder.builder}/${req.buildNumOrId}/data`,
      {headers: {'Authorization': `Bearer ${this.accessToken}`}},
    );
    const buildPageDataText = await res.text();
    const buildPageData = jsonbigint.parse(buildPageDataText, this.reviver);
    return buildPageData as BuildPageData;
  }

  async getRelatedBuilds(bbId: string): Promise<RelatedBuildsData> {
    const fetchUrl = `/related_builds/${bbId}`;
    const res = await fetch(
      fetchUrl,
      {headers: {'Authorization': `Bearer ${this.accessToken}`}},
    );
    const relatedBuildsDataText = await res.text();
    const relatedBuildsData = jsonbigint.parse(relatedBuildsDataText, this.reviver);
    return relatedBuildsData as RelatedBuildsData;
  }
}
