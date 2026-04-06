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

import { fireEvent, render, screen } from '@testing-library/react';

import { WidgetType } from '@/crystal_ball/types';

import { AddWidgetModal } from './add_widget_modal';

describe('AddWidgetModal', () => {
  it('renders modal when open', () => {
    render(
      <AddWidgetModal open={true} onClose={jest.fn()} onAdd={jest.fn()} />,
    );
    expect(screen.getByText('Add Widget')).toBeInTheDocument();
    expect(screen.getByText('Markdown Widget')).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <AddWidgetModal open={false} onClose={jest.fn()} onAdd={jest.fn()} />,
    );
    expect(screen.queryByText('Add Widget')).not.toBeInTheDocument();
  });

  it('calls onAdd when a type is clicked', () => {
    const handleAdd = jest.fn();
    render(
      <AddWidgetModal open={true} onClose={jest.fn()} onAdd={handleAdd} />,
    );

    fireEvent.click(screen.getByText('Markdown Widget'));
    expect(handleAdd).toHaveBeenCalledWith(WidgetType.MARKDOWN);

    fireEvent.click(screen.getByText('Distribution Chart'));
    expect(handleAdd).toHaveBeenCalledWith(
      WidgetType.CHART_INVOCATION_DISTRIBUTION,
    );
  });

  it('calls onClose when Cancel is clicked', () => {
    const handleClose = jest.fn();
    render(
      <AddWidgetModal open={true} onClose={handleClose} onAdd={jest.fn()} />,
    );

    fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    expect(handleClose).toHaveBeenCalledTimes(1);
  });
});
