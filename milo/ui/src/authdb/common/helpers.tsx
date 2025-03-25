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
/* eslint-disable no-useless-escape */

import { isEmail } from 'validator';

import {
  Node,
  PrincipalKind,
  Subgraph,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

// Pattern for a valid group name.
export const nameRe = /^([a-z\-]+\/)?[0-9a-z_\-\.@]{1,100}$/;

// Patterns for valid member identities.
const memberRe = /^((user|bot|project|service|anonymous):)?[\w\.\+\@\%\-\:]+$/;
const anonymousIdentityRe = /^anonymous:anonymous$/;
const botIdentityRe = /^bot:[\w\.\@\-]+$/;
const projectIdentityRe = /^project:[a-z0-9\_\-]+$/;
const serviceIdentityRe = /^service:[\w\.\-\:]+$/;

// Glob pattern should contain at least one '*' and be limited to the superset
// of allowed characters across all kinds.
const globRe = /^[\w\.\+\*\@\%\-\:]*\*[\w\.\+\*\@\%\-\:]*$/;

// Appends '<prefix>:' to a string if it doesn't have a prefix.
const addPrefix = (prefix: string, str: string) => {
  return str.indexOf(':') === -1 ? prefix + ':' + str : str;
};

export const addPrefixToItems = (prefix: string, items: string[]) => {
  return items.map((item) => addPrefix(prefix, item));
};

// True if string looks like a glob pattern.
export const isGlob = (item: string) => {
  return globRe.test(item);
};

export const isMember = (item: string) => {
  if (!memberRe.test(item)) {
    return false;
  }

  switch (item.split(':', 1)[0]) {
    case 'anonymous':
      return anonymousIdentityRe.test(item);
    case 'bot':
      return botIdentityRe.test(item);
    case 'project':
      return projectIdentityRe.test(item);
    case 'service':
      return serviceIdentityRe.test(item);
    case 'user':
    default:
      // The member identity type is "user" (either explicitly,
      // or implicitly assumed when no type is specified).
      // It must be an email.
      return isEmail(item.replace('user:', ''));
  }
};

export const isSubgroup = (item: string) => {
  return nameRe.test(item!);
};

// Strips '<prefix>:' from a string if it starts with it.
export function stripPrefix(prefix: string, value: string) {
  if (!value) {
    return '';
  }
  if (value.startsWith(prefix + ':')) {
    return value.slice(prefix.length + 1, value.length);
  }
  return value;
}

// Applies 'stripPrefix' to each item of a list.
export function stripPrefixFromItems(prefix: string, items: string[]) {
  return items.map((item) => stripPrefix(prefix, item));
}

interface NamedGroup {
  name: string;
  callerCanModify?: boolean;
}

export const getGroupNames = (groups: NamedGroup[]) => {
  return groups.map((g) => g.name);
};

export const toComparableGroupName = (group: NamedGroup) => {
  // Note: callerCanModify is optional; it can be undefined.
  const prefix = group.callerCanModify ? 'A' : 'B';
  const name = group.name;
  if (name.indexOf('/') !== -1) {
    return prefix + 'C' + name;
  }
  if (name.indexOf('-') !== -1) {
    return prefix + 'B' + name;
  }
  return prefix + 'A' + name;
};

export const sortGroupsByName = (groups: NamedGroup[]) => {
  return groups.sort((a: NamedGroup, b: NamedGroup) => {
    const aName = toComparableGroupName(a);
    const bName = toComparableGroupName(b);
    if (aName < bName) {
      return -1;
    } else if (aName > bName) {
      return 1;
    }
    return 0;
  });
};

type Includer = {
  name: string;
  includesDirectly: boolean;
  includesViaGlobs: string[];
  includesIndirectly: string[][];
};

export const interpretLookupResults = (subgraph: Subgraph) => {
  // Note: the principal is always represented by nodes[0] per API guarantee.
  const nodes = subgraph.nodes;
  const principalNode = nodes[0];
  const principal = principalNode.principal;

  const includers = new Map<string, Includer>();
  const getIncluder = (groupName: string) => {
    if (!includers.has(groupName)) {
      includers.set(groupName, {
        name: groupName,
        includesDirectly: false,
        includesViaGlobs: [],
        includesIndirectly: [],
      });
    }
    return includers.get(groupName);
  };

  const enumeratePaths = (
    current: Node[],
    visitCallback: (path: Node[]) => void,
  ) => {
    visitCallback(current);
    const lastNode = current[current.length - 1];
    if (lastNode.includedBy) {
      lastNode.includedBy.forEach((idx) => {
        const node = nodes[idx];
        current.push(node);
        enumeratePaths(current, visitCallback);
        current.pop();
      });
    }
  };

  const visitor = (path: Node[]) => {
    if (path.length === 1) {
      return; // the trivial [principal] path
    }

    const lastNode = path[path.length - 1];
    if (lastNode!.principal!.kind !== PrincipalKind.GROUP) {
      return; // we are only interested in examining groups; skip GLOBs
    }

    const groupIncluder = getIncluder(lastNode!.principal!.name);
    if (path.length === 2) {
      // The entire path is 'principalNode -> lastNode', meaning group
      // 'last' includes the principal directly.
      groupIncluder!.includesDirectly = true;
    } else if (
      path.length === 3 &&
      path[1]!.principal!.kind === PrincipalKind.GLOB
    ) {
      // The entire path is 'principalNode -> GLOB -> lastNode', meaning
      // 'last' includes the principal via the GLOB.
      groupIncluder!.includesViaGlobs.push(
        stripPrefix('user', path[1]!.principal!.name),
      );
    } else {
      // Some arbitrarily long indirect inclusion path. Just record all
      // group names in it (skipping GLOBs). Skip the root principal
      // itself (path[0]) and the currently analyzed node (path[-1]);
      // it's not useful information as it's the same for all paths.
      const groupNames = [];
      for (let i = 1; i < path.length - 1; i++) {
        if (path[i]!.principal!.kind === PrincipalKind.GROUP) {
          groupNames.push(path[i]!.principal!.name);
        }
      }
      groupIncluder!.includesIndirectly.push(groupNames);
    }
  };
  enumeratePaths([principalNode], visitor);

  // Finally, massage the findings for easier display. Note that
  // directIncluders and indirectIncluders are NOT disjoint sets.
  const directIncluders: Includer[] = [];
  const indirectIncluders: Includer[] = [];
  includers.forEach((inc) => {
    if (inc.includesDirectly || inc.includesViaGlobs.length > 0) {
      directIncluders.push(inc);
    }

    if (inc.includesIndirectly.length > 0) {
      // Long inclusion paths look like data dumps in UI and don't fit
      // most of the time. The most interesting components are at the
      // ends, so keep only them.
      inc.includesIndirectly = shortenInclusionPaths(inc.includesIndirectly);
      indirectIncluders.push(inc);
    }
  });

  sortGroupsByName(directIncluders);
  sortGroupsByName(indirectIncluders);

  return {
    principalName: stripPrefix('user', principal!.name),
    principalIsGroup: principal!.kind === PrincipalKind.GROUP,
    includers: includers, // will be used to construct popovers
    directIncluders: directIncluders,
    indirectIncluders: indirectIncluders,
  };
};

const shortenInclusionPaths = (paths: string[][]) => {
  // For each long path in the list, kick out the middle and replace it with ''.
  const out: string[][] = [];
  const seen = new Set();

  paths.forEach((path) => {
    if (path.length <= 3) {
      out.push(path); // short enough already
      return;
    }
    const shorter = [path[0], '', path[path.length - 1]];
    const key = shorter.join('\n');
    if (!seen.has(key)) {
      seen.add(key);
      out.push(shorter);
    }
  });

  return out;
};

export const principalAsRequestProto = (principal: string) => {
  // Normalize the principal to the form the API expects. As everywhere in the UI,
  // if there is no identity type prefix, we assume the 'user:' prefix for emails and globs.
  // In addition, emails all have '@' symbol and (unlike external groups such as google/a@b)
  // don't have '/', and globs all have '*' symbol. Everything else is assumed
  // to be a group.
  const isEmail =
    principal.indexOf('@') !== -1 && principal.indexOf('/') === -1;
  const isGlob = principal.indexOf('*') !== -1;
  if ((isEmail || isGlob) && principal.indexOf(':') === -1) {
    principal = 'user:' + principal;
  }
  let kind = PrincipalKind.GROUP;
  if (isGlob) {
    kind = PrincipalKind.GLOB;
  } else if (isEmail) {
    kind = PrincipalKind.IDENTITY;
  }
  const principalRequest = {
    name: principal,
    kind: kind,
  };
  const request = {
    principal: principalRequest,
  };
  return request;
};
