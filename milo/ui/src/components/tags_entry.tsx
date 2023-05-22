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

import { MobxLitElement } from '@adobe/lit-mobx';
import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache } from '@emotion/react';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { Fragment, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import { StringPair } from '../services/common';
import { commonStyles } from '../styles/stylesheets';
import { ExpandableEntry, ExpandableEntryBody, ExpandableEntryHeader } from './expandable_entry';

export interface TagsEntryProps {
  readonly tags: readonly StringPair[];
}

export function TagsEntry({ tags }: TagsEntryProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={setExpanded}>
        <span css={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
          Tags:
          {!expanded && (
            <span css={{ color: 'var(--greyed-out-text-color)' }}>
              {tags.map((tag, i) => (
                <Fragment key={i}>
                  <span>{' ' + tag.key}</span>: <span>{tag.value}</span>
                  {i !== tags.length - 1 && ','}
                </Fragment>
              ))}
            </span>
          )}
        </span>
      </ExpandableEntryHeader>
      <ExpandableEntryBody ruler="invisible">
        <table
          css={{ width: 'fit-content', overflow: 'hidden' }}>
          <tbody>
            {tags.map((tag, i) => (
              <tr key={i}>
                <td>{tag.key}:</td>
                <td>
                  {tag.value?.match(/^https?:\/\//i) ? (
                    <a href={tag.value} target="_blank">
                      {tag.value}
                    </a>
                  ) : (
                    tag.value
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}

@customElement('milo-tags-entry')
export class DotSpinnerElement extends MobxLitElement {
  @observable.ref tags!: readonly StringPair[];

  private readonly cache: EmotionCache;
  private readonly parent: HTMLSpanElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('span');
    const child = document.createElement('span');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-tags-entry',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <TagsEntry tags={this.tags} />
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyles];
}
