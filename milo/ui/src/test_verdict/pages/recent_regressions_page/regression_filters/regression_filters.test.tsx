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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import { ChangepointPredicate } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';

import { RegressionFilters } from './regression_filters';

describe('<RegressionFilters />', () => {
  afterEach(() => {
    cleanup();
  });

  it('only update predicate when apply filter button is clicked', () => {
    const onPredicateChangeSpy = jest.fn((_: ChangepointPredicate) => {});
    render(
      <RegressionFilters
        predicate={ChangepointPredicate.fromPartial({
          testIdContain: 'contain1',
        })}
        onPredicateUpdate={onPredicateChangeSpy}
      />,
    );

    const containInputEle = screen.getByLabelText('Test ID contain');
    expect(containInputEle).toHaveValue('contain1');

    // Input search query.
    fireEvent.change(containInputEle, { target: { value: 'contain-new' } });
    expect(containInputEle).toHaveValue('contain-new');
    expect(onPredicateChangeSpy).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText('Apply Filter'));
    expect(onPredicateChangeSpy).toHaveBeenCalledTimes(1);
    expect(onPredicateChangeSpy).toHaveBeenNthCalledWith(
      1,
      ChangepointPredicate.fromPartial({ testIdContain: 'contain-new' }),
    );
  });

  it('can override pending value when parent set an update', () => {
    const onPredicateChangeSpy = jest.fn((_: ChangepointPredicate) => {});
    const { rerender } = render(
      <RegressionFilters
        predicate={ChangepointPredicate.fromPartial({
          testIdContain: 'contain1',
        })}
        onPredicateUpdate={onPredicateChangeSpy}
      />,
    );

    const containInputEle = screen.getByLabelText('Test ID contain');
    expect(containInputEle).toHaveValue('contain1');

    // Input search query.
    fireEvent.change(containInputEle, { target: { value: 'contain-new' } });
    expect(containInputEle).toHaveValue('contain-new');

    // Parent sets a new value while pending.
    rerender(
      <RegressionFilters
        predicate={ChangepointPredicate.fromPartial({
          testIdContain: 'contain2',
        })}
        onPredicateUpdate={onPredicateChangeSpy}
      />,
    );
    expect(containInputEle).toHaveValue('contain2');

    // No pending update since it was discarded.
    expect(screen.getByText('Apply Filter')).toBeDisabled();
  });
});
