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

import {
  UseMutationOptions,
  UseMutationResult,
  UseQueryResult,
} from '@tanstack/react-query';

import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';

import { useGapiMutation, useGapiQuery } from '../gapi_query';

// Root path for the API, matching your gapi_query example pattern
const API_BASE_PATH_ROOT = 'https://androidengprod-pa.googleapis.com/';
// Product-specific path for this service
const API_PRODUCT_PATH = '/v1/growler/';

// Note that many of the types in this file are manually defined from the proto
// as this is a private API and we only define what we need.

// ===================================================================
// Shared Types
// ===================================================================

export interface BugId {
  /**
   * Id of a Buganizer bug.
   * Proto is int64, using string for JSON safety.
   */
  buganizer_bug_id: string;
}

export interface GrowlerIssue {
  /**
   * Output only. Unique identifier of this issue created by Growler.
   */
  issue_id: string;
  /**
   * Name of the issue. This is the full resource identifier.
   * E.g. "android/internal/engprod/growler/v1/issues/12345"
   */
  name: string;
  /**
   * The ID of the bug for this issue.
   */
  bug_id?: BugId;
}

// ===================================================================
// GetIssue
// ===================================================================

export interface GetIssueParams {
  /**
   * The Growler issue identifier.
   * E.g. "12345"
   */
  issueId: string;
}

/**
 * Gets a single Growler issue by its ID.
 */
export const useGetIssue = (
  params: GetIssueParams,
  queryOptions: WrapperQueryOptions<GrowlerIssue>,
): UseQueryResult<GrowlerIssue> => {
  return useGapiQuery<GrowlerIssue>(
    {
      method: 'GET',
      path: `${API_BASE_PATH_ROOT}${API_PRODUCT_PATH}issues/${params.issueId}`,
    },
    queryOptions,
  );
};

// ===================================================================
// SearchIssues
// ===================================================================

export interface SearchIssuesQuery {
  index?: number;
  bug_id?: BugId;
}

export interface SearchIssuesRequest {
  search_issues_queries: SearchIssuesQuery[];
}

export interface SearchIssuesResult {
  index?: number;
  issues: GrowlerIssue[];
}

export interface SearchIssuesResponse {
  search_issues_results: SearchIssuesResult[];
}

/**
 * Searches for matching issues. This uses a POST request to send the query body.
 */
export const useSearchIssues = (
  body: SearchIssuesRequest,
  queryOptions: WrapperQueryOptions<SearchIssuesResponse>,
): UseQueryResult<SearchIssuesResponse> => {
  return useGapiQuery<SearchIssuesResponse>(
    {
      method: 'POST',
      path: `${API_BASE_PATH_ROOT}${API_PRODUCT_PATH}issues:search`,
      body,
    },
    queryOptions,
  );
};

// ===================================================================
// SearchRelatedIssues
// ===================================================================

export interface SearchRelatedIssuesQuery {
  index?: number;
}

export interface SearchRelatedIssuesRequest {
  search_related_issues_queries: SearchRelatedIssuesQuery[];
}

export interface RelatedIssue {
  issue: GrowlerIssue;
}

export interface SearchRelatedIssuesResult {
  index?: number;
  related_issues: RelatedIssue[];
}

export interface SearchRelatedIssuesResponse {
  search_related_issues_results: SearchRelatedIssuesResult[];
}

/**
 * Searches for related issues. This uses a POST request to send the query body.
 */
export const useSearchRelatedIssues = (
  body: SearchRelatedIssuesRequest,
  queryOptions: WrapperQueryOptions<SearchRelatedIssuesResponse>,
): UseQueryResult<SearchRelatedIssuesResponse> => {
  return useGapiQuery<SearchRelatedIssuesResponse>(
    {
      method: 'POST',
      path: `${API_BASE_PATH_ROOT}${API_PRODUCT_PATH}issues:searchRelated`,
      body,
    },
    queryOptions,
  );
};

// ===================================================================
// CreateIssue (Mutation)
// ===================================================================

export interface CreateIssueRequest {
  /**
   * The issue to be created.
   * Note: On create, many fields like issue_id/name will be empty.
   */
  issue: Partial<GrowlerIssue>;
  reporter_quota_user_id?: string;
}

/**
 * Mutation hook to create a new Growler issue.
 */
export const useCreateIssue = (
  mutationOptions?: UseMutationOptions<GrowlerIssue, Error, CreateIssueRequest>,
): UseMutationResult<GrowlerIssue, Error, CreateIssueRequest> => {
  return useGapiMutation<CreateIssueRequest, GrowlerIssue>(
    (request) => ({
      method: 'POST',
      path: `${API_BASE_PATH_ROOT}${API_PRODUCT_PATH}issues`,
      body: request,
    }),
    mutationOptions,
  );
};

// ===================================================================
// UpdateIssue (Mutation)
// ===================================================================

export interface UpdateIssueRequest {
  /**
   * The issue which replaces the issue on the Growler server.
   * The issue.name MUST contain the full resource name to identify it.
   */
  issue: GrowlerIssue;
  /**
   * Field mask, e.g. "labels,metadata"
   */
  update_mask: string;
}

/**
 * Mutation hook to update an existing Growler issue.
 */
export const useUpdateIssue = (
  mutationOptions?: UseMutationOptions<GrowlerIssue, Error, UpdateIssueRequest>,
): UseMutationResult<GrowlerIssue, Error, UpdateIssueRequest> => {
  return useGapiMutation<UpdateIssueRequest, GrowlerIssue>(
    (request) => ({
      method: 'POST',
      path: `${API_BASE_PATH_ROOT}${request.issue.name}`,
      params: { update_mask: request.update_mask },
      body: request.issue,
    }),
    mutationOptions,
  );
};
