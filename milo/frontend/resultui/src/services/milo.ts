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

import { Build, BuilderID, Step } from './buildbucket';

export interface GetBuildPageDataRequest {
  builder: BuilderID;
  number: number;
}

export interface BuildPageData extends Build {
  now: string;
  blame: Commit[];
  build_bug_link: string;
  buildbucket_host: string;
  // errors: unknown[];
  // step_display_pref: string;
  // show_debug_logs_pref: boolean;
  // blamelist_error: unknown;
  // forced_blameList: boolean;
  can_cancel: boolean;
  can_retry: boolean;

	commit_link_html: string;
  summary: string[];
  // change_links: Link[];
  recipe_link: Link;
  buildbucket_link: Link;
  build_sets: string[];
  buildset_links: string[];
  steps: StepExt[];
  human_status: string;
  should_show_canary_warming: boolean;
  input_properties: Property[];
  output_properties: Property[];
  builder_link: Link;
  link: Link;
  banners: Logo[];
  // revision_html: string;
  timeline: string;
  // test_results_opt_in_html: string;
}

export interface Commit {
 author_name: string;
 author_email: string;
 commit_time: string;
 repo: string;
 branch: string;
 request_revision: Link;
 revision: Link;
 description: string;
 changelist: Link;
 commit_url: string;
 file: string[];
}

export interface Property {
  name: string;
  value: string;
}

export interface StepExt extends Step {
  children: StepExt[];
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
  aria_label: string;
  img: string;
  alt: string;
  alias: boolean;
}

export interface LogoBase {
  img: string;
  alt: string;
}

export interface Logo extends LogoBase {
  subtitle: string;
  count: number;
}

export class Milo {
  constructor(readonly host: string, private accessToken: string) {
  }

  async getBuildPageData(req: GetBuildPageDataRequest) {
    const res = await fetch(
      `${this.host}/p/${req.builder.project}/builders/${req.builder.bucket}/${req.builder.builder}/${req.number}/data`,
      {headers: {'Authorization': `Bearer ${this.accessToken}`}},
    );
    const buildPageData = await res.json();
    return buildPageData as BuildPageData;
  }
}
