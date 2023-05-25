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

import { expect } from '@jest/globals';
import { fireEvent, render, screen } from '@testing-library/react';

import { TagsEntry } from './tags_entry';

const tags = [
  { key: 'key1', value: 'val1' },
  { key: 'key2', value: 'https://www.example.com' },
];

describe('TagsEntry', () => {
  it('e2e', () => {
    const { rerender } = render(<TagsEntry tags={tags} />);

    const tagsEle = screen.getByText<HTMLSpanElement>('tags:', { exact: false });

    // Tags not displayed in header when the entry is collapsed.
    expect(tagsEle.textContent).toStrictEqual('Tags: key1: val1, key2: https://www.example.com');
    expect(screen.queryByRole('table')).toBeNull();

    // Expand the tags entry.
    fireEvent.click(tagsEle);
    rerender(<TagsEntry tags={tags} />);

    // Tags displayed in header when the entry is expanded.
    expect(tagsEle.textContent).toStrictEqual('Tags:');
    expect(screen.queryByRole('table')).not.toBeNull();
    expect(screen.getByText<HTMLElement>('val1').tagName).not.toStrictEqual('A');
    expect(screen.getByText<HTMLElement>('https://www.example.com').tagName).toStrictEqual('A');

    // Collapse the tags entry.
    fireEvent.click(tagsEle);
    rerender(<TagsEntry tags={tags} />);

    // Tags not displayed in header when the entry is collapsed.
    expect(tagsEle.textContent).toStrictEqual('Tags: key1: val1, key2: https://www.example.com');
    expect(screen.queryByRole('table')).toBeNull();
  });
});
