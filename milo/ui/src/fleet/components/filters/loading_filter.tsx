// Copyright 2026 The LUCI Authors.
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

import * as ast from '@/fleet/utils/aip160/ast/ast';

import { FilterCategory } from './use_filters';

export class LoadingFilterCategory implements FilterCategory {
  public label: string;
  public key: string;
  private chipLabel: string;

  constructor(key: string) {
    this.label = key;
    this.key = key;
    this.chipLabel = key;
  }

  public clone() {
    const newInst = new LoadingFilterCategory(this.label);
    return newInst as FilterCategory;
  }

  public fromAIP160(terms: (ast.Term & { simple: ast.Restriction })[]) {
    const opt = terms.map((t) => {
      if (t.negated) return '(Blank)';

      const member =
        t.simple.arg?.kind === 'Comparable'
          ? t.simple.arg?.member
          : t.simple.comparable.member;

      return member.value.value;
    });

    this.chipLabel = `${terms.length} | [ ${this.key} ]: ${opt.join(', ')} `;
  }

  public toAIP160(): string {
    return '';
  }

  public render(
    _childrenSearchQuery: string,
    _onNavigateUp: (e: React.KeyboardEvent) => void,
    _onApply: () => void,
    _onClose: () => void,
    _ref?: React.Ref<unknown>,
  ) {
    //TODO:
    return 'loading...';
  }
  public getChipLabel() {
    return this.chipLabel;
  }

  public isActive() {
    return true;
  }
  public clear() {}

  public getChildrenSearchScore(_searchQuery: string) {
    return 0;
  }
}
