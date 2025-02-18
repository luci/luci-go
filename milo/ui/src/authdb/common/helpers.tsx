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

import validator from 'validator';

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
      return validator.isEmail(item.replace('user:', ''));
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
