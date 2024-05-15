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
import { ChevronRight, ExpandMore } from '@mui/icons-material';
import { Box, SxProps, Theme } from '@mui/material';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { makeObservable, observable } from 'mobx';
import { createContext, useContext } from 'react';

const ExpandedContext = createContext(false);

export interface ExpandableEntryHeaderProps {
  readonly onToggle: (expand: boolean) => void;
  readonly sx?: SxProps<Theme>;
  readonly children: React.ReactNode;
}

/**
 * Renders the header of an <ExpandableEntry />.
 */
export function ExpandableEntryHeader({
  onToggle,
  sx,
  children,
}: ExpandableEntryHeaderProps) {
  const expanded = useContext(ExpandedContext);

  return (
    <Box
      onClick={() => onToggle(!expanded)}
      sx={{
        display: 'grid',
        gridTemplateColumns: '24px 1fr',
        gridTemplateRows: '24px',
        gridGap: '5px',
        cursor: 'pointer',
        lineHeight: '24px',
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        ...sx,
      }}
    >
      {expanded ? <ExpandMore /> : <ChevronRight />}
      {children}
    </Box>
  );
}

export interface ExpandableEntryBodyProps {
  /**
   * Configure whether the content ruler should be rendered.
   * * visible: the default option. Renders the content ruler.
   * * invisible: hide the content ruler but keep the indentation.
   * * none: hide the content ruler and don't keep the indentation.
   */
  readonly ruler?: 'visible' | 'invisible' | 'none';
  readonly children: React.ReactNode;
}

/**
 * Renders the body of an <ExpandableEntry />.
 * The content is hidden when the entry is collapsed.
 */
export function ExpandableEntryBody({
  ruler,
  children,
}: ExpandableEntryBodyProps) {
  const expanded = useContext(ExpandedContext);
  ruler = ruler || 'visible';

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: ruler === 'none' ? '1fr' : '24px 1fr',
        gridGap: '5px',
      }}
    >
      <Box
        sx={{
          display: ruler === 'none' ? 'none' : '',
          visibility: ruler === 'invisible' ? 'hidden' : '',
          borderLeft: '1px solid var(--divider-color)',
          width: '0px',
          marginLeft: '11.5px',
        }}
      ></Box>
      {expanded ? children : <></>}
    </Box>
  );
}

export interface ExpandableEntryProps {
  readonly expanded: boolean;
  readonly sx?: SxProps<Theme>;
  /**
   * The first child should be an <ExpandableEntryHeader />.
   * The second child should be an <ExpandableEntryBody />.
   */
  readonly children: [JSX.Element, JSX.Element];
}

/**
 * Renders an expandable entry.
 */
export function ExpandableEntry({
  expanded,
  sx,
  children,
}: ExpandableEntryProps) {
  return (
    <Box sx={sx}>
      <ExpandedContext.Provider value={expanded}>
        {children}
      </ExpandedContext.Provider>
    </Box>
  );
}

/**
 * Renders an expandable entry.
 */
// Keep a separate implementation instead of wrapping the React component so
// 1. we can catch events originated from shadow-dom, and
// 2. the rendering performance is as good as possible (there could be > 10,000
// entries rendered on the screen).
@customElement('milo-expandable-entry')
export class ExpandableEntryElement extends MobxLitElement {
  /**
   * Configure whether the content ruler should be rendered.
   * * visible: the default option. Renders the content ruler.
   * * invisible: hide the content ruler but keep the indentation.
   * * none: hide the content ruler and don't keep the indentation.
   */
  @observable.ref contentRuler: 'visible' | 'invisible' | 'none' = 'visible';

  onToggle = (_isExpanded: boolean) => {
    /* do nothing by default */
  };

  @observable.ref private _expanded = false;
  get expanded() {
    return this._expanded;
  }
  set expanded(isExpanded) {
    if (isExpanded === this._expanded) {
      return;
    }
    this._expanded = isExpanded;
    this.onToggle(this._expanded);
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <div
        id="expandable-header"
        @click=${() => (this.expanded = !this.expanded)}
      >
        <mwc-icon>${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <slot name="header"></slot>
      </div>
      <div
        id="body"
        style=${styleMap({
          'grid-template-columns':
            this.contentRuler === 'none' ? '1fr' : '24px 1fr',
        })}
      >
        <div
          id="content-ruler"
          style=${styleMap({
            display: this.contentRuler === 'none' ? 'none' : '',
            visibility: this.contentRuler === 'invisible' ? 'hidden' : '',
          })}
        ></div>
        <slot
          name="content"
          style=${styleMap({ display: this.expanded ? '' : 'none' })}
        ></slot>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: block;
      --header-height: 24px;
    }

    #expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: var(--header-height);
      grid-gap: 5px;
      cursor: pointer;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      border-left: 1px solid var(--divider-color);
      width: 0px;
      margin-left: 11.5px;
    }
  `;
}
