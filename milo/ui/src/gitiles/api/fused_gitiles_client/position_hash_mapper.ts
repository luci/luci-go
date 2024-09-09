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

import { NEVER_PROMISE } from '@/common/constants/utils';

/**
 * Maps a commit position to a commit hash.
 */
interface ResolvedMapping {
  readonly position: number;
  readonly hash: string;
}

/**
 * Maps a commit position to a commit hash promise.
 */
interface UnresolvedMapping {
  readonly position: number;
  readonly hash: Promise<string>;
}

const NO_RESOLVED_MAPPING: ResolvedMapping = {
  position: -Infinity,
  hash: '',
};

const NO_UNRESOLVED_MAPPING: UnresolvedMapping = {
  position: -Infinity,
  hash: NEVER_PROMISE,
};

/**
 * A class that helps mapping commit positions to commit hashes without sending
 * too many queries.
 */
export class PositionHashMapper {
  private resolvedMapping = NO_RESOLVED_MAPPING;
  private unresolvedMapping = NO_UNRESOLVED_MAPPING;

  constructor(
    private readonly getCommitHash: (position: number) => Promise<string>,
  ) {}

  /**
   * Returns a promise that resolves to a commitish that maps to the given
   * commit hash.
   */
  async getCommitish(position: number): Promise<string> {
    // If we already have a commit position from a descendant commit, compute
    // the commitish from there.
    if (this.resolvedMapping.position >= position) {
      return `${this.resolvedMapping.hash}~${this.resolvedMapping.position - position}`;
    }

    // If we haven't sent a query to get the commit hash of a descendant commit,
    // send one now.
    if (this.unresolvedMapping.position < position) {
      const unresolvedMapping = {
        position,
        hash: this.getCommitHash(position).then((hash) => {
          // Once the query resolves, update the resolved mapping so we can
          // support getting commitish for larger commit positions without
          // sending a query.
          if (this.resolvedMapping.position < position) {
            this.resolvedMapping = { position, hash };
          }
          return hash;
        }),
      };
      this.unresolvedMapping = unresolvedMapping;

      // If the query failed, reset the unresolved mapping so new queries can be
      // sent.
      unresolvedMapping.hash.catch((_e) => {
        if (this.unresolvedMapping === unresolvedMapping) {
          this.unresolvedMapping = NO_UNRESOLVED_MAPPING;
        }
      });
    }

    return `${await this.unresolvedMapping.hash}~${this.unresolvedMapping.position - position}`;
  }
}
