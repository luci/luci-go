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
  UseQueryOptions,
  UseQueryResult,
  UseInfiniteQueryOptions,
} from '@tanstack/react-query';

import { useGapiQuery, useInfiniteGapiQuery } from '../gapi_query';

const API_BASE_PATH = 'https://content-issuetracker.corp.googleapis.com/';

// Note that many of the types in this file can be auto-generated, but are not.
// This is because this is a private API and we only define what we need for our code.

/**
 * Gets the current state of a single component
 */
export const useGetComponentQuery = (
  params: GetComponentParams,
  queryOptions: UseQueryOptions<ComponentJson>,
): UseQueryResult<ComponentJson> => {
  return useGapiQuery<ComponentJson>(
    {
      method: 'GET',
      path: API_BASE_PATH + `v1/components/${params.componentId}`,
    },
    queryOptions,
  );
};

/**
 * Gets the current state of a single issue
 */
export const useGetIssueQuery = (
  params: GetIssueParams,
  queryOptions: UseQueryOptions<IssueJson>,
): UseQueryResult<IssueJson> => {
  return useGapiQuery<IssueJson>(
    {
      method: 'GET',
      path: API_BASE_PATH + `v1/issues/${params.issueId}`,
    },
    queryOptions,
  );
};

/**
 * Searches issues in the search cache (which may be stale), then returns the
 * current state of the matched issues (which may no longer match
 * query and may no longer be in the order indicated by order_by).
 */
export const useIssueListQuery = (
  params: IssueListParams,
  queryOptions: UseQueryOptions<IssueListResponse>,
): UseQueryResult<IssueListResponse> => {
  return useGapiQuery<IssueListResponse>(
    {
      method: 'GET',
      path: API_BASE_PATH + 'v1/issues',
      params,
    },
    queryOptions,
  );
};

/**
 * Similar to useIssueListQuery but allows for infnite loading of issues.
 */
export function useInfiniteIssueListQuery(
  params: IssueListParams,
  queryOptions: UseInfiniteQueryOptions<IssueListResponse>,
) {
  return useInfiniteGapiQuery<IssueListResponse>(
    {
      method: 'GET',
      path: API_BASE_PATH + 'v1/issues',
      params,
    },
    queryOptions,
  );
}

export interface GetIssueParams {
  /**
   * The id of the issue to get.
   * E.g. "1234"
   */
  issueId: string;
}

export interface IssueListParams {
  /**
   * Query language for issues requests is defined at:
   * https://developers.google.com/issue-tracker/concepts/search-query-language.
   * Issues returned in the response will have matched this query at some point
   * in the hopefully-recent past.
   */
  query?: string;

  /**
   * Order parameter.  Order is ascending ("asc") by default, but can
   * be defined with the sort field followed by a space and "asc" or
   * "desc".
   *
   * Examples:
   *     "issue_id",
   *     "modified_time asc",
   *
   * (Note that the issues are ordered based on a state that's possibly less
   * recent than the one that's returned, so the returned issues may not be in
   * the indicated order if they're ordered based on a property which can
   * change.)
   *
   * Secondary sorts may be specified in comma-separated format.
   *
   * Examples:
   *    "priority asc, created_time desc"
   *    "custom_field:1234, modified_time desc"
   *
   * Valid sort fields are:
   *
   *   * archived
   *   * assignee
   *   * cc_count
   *   * component_path
   *   * created_time
   *   * custom_field:<id>
   *   * descendant_count
   *   * deletion_time
   *   * duplicate_count
   *   * found_in_versions
   *   * in_prod
   *   * issue_id
   *   * last_modifier
   *   * modified_time
   *   * one_day_view_count
   *   * open_descendant_count
   *   * priority
   *   * reporter
   *   * seven_day_view_count
   *   * severity
   *   * status
   *   * targeted_to_versions
   *   * thirty_day_view_count
   *   * title
   *   * type
   *   * verified_in_versions
   *   * verified_time
   *   * verifier
   *   * vote_count
   *   * access_level (derived from the issue access limit field)
   */
  orderBy?: string;

  /**
   * Default page_size = 25.
   * Maximum page_size = 500.
   */
  pageSize?: number;

  /**
   * Pagination token.
   */
  pageToken?: string;
}

export interface IssueListResponse {
  /**
   * The current page of issues.
   */
  issues: IssueJson[];

  /**
   * Pagination token for next page of results.
   */
  nextPageToken: string;

  /**
   * Total number of results. This is an approximation.
   */
  totalSize: number;
}

export interface IssueJson {
  /**
   * Unique ID. Assigned at creation time by the API backend.
   */
  issueId: string;

  /**
   * The current state of the issue.  Will always be present.
   */
  issueState: IssueStateJson;
}

export interface IssueStateJson {
  /**
   * Short summary of the issue.
   */
  title: string;

  /**
   * The id of the component the issue belongs to.
   * E.g. "1234"
   */
  componentId: string;

  /**
   * The current priority of the issue.
   * E.g. "P2"
   */
  priority: string;

  /**
   * The current state of the issue.
   * E.g. "OBSOLETE"
   */
  status: string;

  /**
   * The current severity of the issue.
   * E.g. "S2"
   */
  severity: string;

  /**
   * The issue type.
   * E.g. "BUG"
   */
  type: string;

  /**
   * The user that the issue is assigned to.
   */
  assignee?: IssueUserJson;
}

export interface IssueUserJson {
  /**
   * The email address of the user.
   */
  emailAddress: string;
}

export interface GetComponentParams {
  /**
   * The id of the component to get.
   * E.g. "1234"
   */
  componentId: string;
}

export interface ComponentJson {
  /**
   * The path info of the component.
   */
  componentPathInfo: ComponentPathInfoJson;
}

export interface ComponentPathInfoJson {
  /**
   * The names of this component and all of it's ancestor components in order.
   * E.g. ["grandparent", "parent", "example component"]
   */
  componentPathNames: string[];
}
