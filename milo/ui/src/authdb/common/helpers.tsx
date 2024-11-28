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

import validator from 'validator';

export const nameRe = /^([a-z\-]+\/)?[0-9a-z_\-\.@]{1,100}$/;
export const membersRe = /^((user|bot|project|service|anonymous):)?[\w+%.@*\[\]-]+$/;
export const nonUserMemberRe = /^(bot|project|service|anonymous):.+/;

// Glob pattern should contain at least one '*' and be limited to the superset
// of allowed characters across all kinds.
const globRe = /^[\w\.\+\*\@\%\-\:]*\*[\w\.\+\*\@\%\-\:]*$/;

// Appends '<prefix>:' to a string if it doesn't have a prefix.
const addPrefix = (prefix: string, str: string) => {
    return str.indexOf(':') == -1 ? prefix + ':' + str : str;
};

export const addPrefixToItems = (prefix: string, items: string[]) => {
    return items.map((item) => addPrefix(prefix, item));
};

// True if string looks like a glob pattern.
export const isGlob = (item: string) => {
    return globRe.test(item);
};

export const isMember = (item: string) => {
    return membersRe.test(item!) && isValidMember(item!);
}

export const isSubgroup = (item: string) => {
    return nameRe.test(item!);
}

const isValidMember = (item: string) => {
    // Non user members are valid as long as prefix is specified.
    if (nonUserMemberRe.test(item!)) {
        return true;
    }
    // The member identity type is "user" (either explicitly,
    // or implicitly assumed when no type is specified).
    // It must be an email.
    return validator.isEmail(item.replace('user:', ''));
}