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

import { styled } from '@mui/material';
import { ReactNode, useEffect, useRef } from 'react';

import { OptionDef } from './types';

const Row = styled('tr')`
  --light-hover-color: #eef8ff;

  &.selectable {
    color: var(--active-color);
  }

  &.selected {
    border-color: var(--light-active-color);
    background-color: var(--light-active-color);
  }

  &.selectable:not(.selected):hover {
    border-color: var(--light-hover-color);
    background-color: var(--light-hover-color);
  }

  & > td {
    overflow: hidden;
  }
`;

export interface OptionRowProps<T> {
  readonly def: OptionDef<T>;
  readonly selected?: boolean;

  readonly onClick: () => void;
  readonly children: ReactNode;
}

export function OptionRow<T>({
  def,
  selected = false,
  onClick,
  children,
}: OptionRowProps<T>) {
  const ref = useRef<HTMLTableRowElement>(null);

  useEffect(() => {
    if (selected) {
      ref.current?.scrollIntoView({ block: 'nearest' });
    }
  }, [selected]);

  const selectableClass = def.unselectable ? '' : 'selectable';
  const selectedClass = selected && !def.unselectable ? 'selected' : '';

  return (
    <Row
      ref={ref}
      onClick={def.unselectable ? undefined : onClick}
      className={`${selectableClass} ${selectedClass}`}
    >
      {children}
    </Row>
  );
}
