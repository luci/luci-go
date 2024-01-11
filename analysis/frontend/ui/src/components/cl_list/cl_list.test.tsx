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

import '@testing-library/jest-dom';

import {
  fireEvent,
  render,
  screen,
  waitForElementToBeRemoved,
} from '@testing-library/react';

import { Changelist } from '@/legacy_services/shared_models';

import CLList from './cl_list';

describe('Test CLList component', () => {
  it('given no CLs', async () => {
    const { container } = render(
        <CLList changelists={[]}/>,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('given a single CL', async () => {
    const changelist: Changelist = {
      host: 'chromium-review.googlesource.com',
      change: '12345678901234',
      patchset: 543,
    };
    render(
        <CLList changelists={[changelist]}/>,
    );

    await screen.findByText('12345678901234 #543');

    expect(screen.getByText('12345678901234 #543')).toBeInTheDocument();
    expect(screen.queryByText('...')).toBeNull();
  });

  it('given a multiple CLs', async () => {
    const changelists: Changelist[] = [{
      host: 'chromium-review.googlesource.com',
      change: '12345678901234',
      patchset: 543,
    }, {
      host: 'chromium-internal-review.googlesource.com',
      change: '85132217',
      patchset: 432,
    }];
    render(
        <CLList changelists={changelists}/>,
    );

    expect(await screen.findByText('12345678901234 #543')).toBeInTheDocument();
    expect(screen.getByText('...')).toBeInTheDocument();
    expect(screen.queryByText('85132217 #432')).toBeNull();

    fireEvent.click(screen.getByText('...'));

    expect(await screen.findByText('85132217 #432')).toBeInTheDocument();

    fireEvent.click(screen.getByText('...'));

    await waitForElementToBeRemoved(() => screen.queryByText('85132217 #432'));
    expect(screen.queryByText('85132217 #432')).toBeNull();
  });
});
