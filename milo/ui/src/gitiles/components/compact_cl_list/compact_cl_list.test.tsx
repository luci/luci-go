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

import { render, screen } from '@testing-library/react';

import { GerritChange } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { CompactClList } from './compact_cl_list';

const changes = [
  GerritChange.fromPartial({
    host: 'host.googlesource.com',
    project: 'my-project',
    change: '1234',
    patchset: '1',
  }),
  GerritChange.fromPartial({
    host: 'host.googlesource.com',
    project: 'my-project',
    change: '2345',
    patchset: '2',
  }),
  GerritChange.fromPartial({
    host: 'host.googlesource.com',
    project: 'my-project',
    change: '3456',
    patchset: '3',
  }),
];

describe('CompactClList', () => {
  it('can render Cls', () => {
    render(<CompactClList changes={changes} />);

    expect(screen.queryByText('1234#1')).toBeInTheDocument();
    expect(screen.queryByText('2345#2')).not.toBeInTheDocument();
    expect(screen.queryByText('3456#3')).not.toBeInTheDocument();
    expect(screen.queryByText('+ 2 more')).toBeInTheDocument();
  });

  it('can render multiple inline Cls', () => {
    render(<CompactClList changes={changes} maxInlineCount={2} />);

    expect(screen.queryByText('1234#1')).toBeInTheDocument();
    expect(screen.queryByText('2345#2')).toBeInTheDocument();
    expect(screen.queryByText('3456#3')).not.toBeInTheDocument();
    expect(screen.queryByText('+ 1 more')).toBeInTheDocument();
  });

  it('when maxInlineCount is greater than the number of CLs', () => {
    render(<CompactClList changes={changes} maxInlineCount={4} />);

    expect(screen.queryByText('1234#1')).toBeInTheDocument();
    expect(screen.queryByText('2345#2')).toBeInTheDocument();
    expect(screen.queryByText('3456#3')).toBeInTheDocument();
    expect(
      screen.queryByText('more', { exact: false }),
    ).not.toBeInTheDocument();
  });
});
