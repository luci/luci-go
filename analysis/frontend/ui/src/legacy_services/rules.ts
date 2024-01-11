// Copyright 2022 The LUCI Authors.
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

import { AuthorizedPrpcClient } from '@/clients/authorized_client';

import {
  AssociatedBug,
  ClusterId,
} from './shared_models';

export const getRulesService = () : RulesService => {
  const client = new AuthorizedPrpcClient();
  return new RulesService(client);
};

// For handling errors, import:
// import { GrpcError } from '@chopsui/prpc-client';
export class RulesService {
  private static SERVICE = 'luci.analysis.v1.Rules';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async get(request: GetRuleRequest) : Promise<Rule> {
    return this.client.call(RulesService.SERVICE, 'Get', request, {});
  }

  async list(request: ListRulesRequest): Promise<ListRulesResponse> {
    return this.client.call(RulesService.SERVICE, 'List', request, {});
  }

  async create(request: CreateRuleRequest): Promise<Rule> {
    return this.client.call(RulesService.SERVICE, 'Create', request, {});
  }

  async update(request: UpdateRuleRequest): Promise<Rule> {
    return this.client.call(RulesService.SERVICE, 'Update', request, {});
  }

  async lookupBug(request: LookupBugRequest): Promise<LookupBugResponse> {
    return this.client.call(RulesService.SERVICE, 'LookupBug', request, {});
  }
}

export interface GetRuleRequest {
  // The name of the rule to retrieve.
  // Format: projects/{project}/rules/{rule_id}.
  name: string;
}

export interface Rule {
  name: string;
  project: string;
  ruleId: string;
  ruleDefinition?: string;
  bug: AssociatedBug;
  isActive: boolean;
  isManagingBug: boolean;
  isManagingBugPriority: boolean;
  sourceCluster: ClusterId;
  bugManagementState: BugManagementState;
  createTime: string; // RFC 3339 encoded date/time.
  createUser?: string;
  lastAuditableUpdateTime: string; // RFC 3339 encoded date/time.
  lastAuditableUpdateUser?: string;
  lastUpdateTime: string; // RFC 3339 encoded date/time.
  predicateLastUpdateTime: string; // RFC 3339 encoded date/time.
  etag: string;
}

export interface BugManagementState {
  policyState?: PolicyState[];
}

export interface PolicyState {
  policyId: string;
  isActive?: boolean;
  lastActivationTime?: string; // RFC 3339 encoded date/time.
  lastDeactivationTime?: string; // RFC 3339 encoded date/time.
}

export interface ListRulesRequest {
  // The parent, which owns this collection of rules.
  // Format: projects/{project}.
  parent: string;
}

export interface ListRulesResponse {
  rules?: Rule[];
}

export interface CreateRuleRequest {
  parent: string;
  rule: RuleToCreate;
}

export interface RuleToCreate {
  ruleDefinition: string;
  bug: AssociatedBugToUpdate;
  isActive?: boolean;
  isManagingBug?: boolean;
  isManagingBugPriority?: boolean;
  sourceCluster?: ClusterId;
}

export interface AssociatedBugToUpdate {
  system: string;
  id: string;
}

export interface UpdateRuleRequest {
  rule: RuleToUpdate;
  // Comma separated list of fields to be updated.
  // e.g. ruleDefinition,bug,isActive.
  updateMask: string;
  etag?: string;
}

export interface RuleToUpdate {
  name: string;
  ruleDefinition?: string;
  bug?: AssociatedBugToUpdate;
  isActive?: boolean;
  isManagingBug?: boolean;
  isManagingBugPriority?: boolean;
  sourceCluster?: ClusterId;
}

export interface LookupBugRequest {
  system: string;
  id: string;
}

export interface LookupBugResponse {
  // The looked up rules.
  // Format: projects/{project}/rules/{rule_id}.
  rules?: string[];
}

const ruleNameRE = /^projects\/(.*)\/rules\/(.*)$/;

// RuleKey represents the key parts of a rule resource name.
export interface RuleKey {
  project: string;
  ruleId: string;
}

// parseRuleName parses a rule resource name into its key parts.
export const parseRuleName = (name: string):RuleKey => {
  const results = name.match(ruleNameRE);
  if (results == null) {
    throw new Error('invalid rule resource name: ' + name);
  }
  return {
    project: results[1],
    ruleId: results[2],
  };
};
