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

import { LinkifiedText } from './linkified_text';

it('displays normal text', async () => {
  render(<LinkifiedText text="abcd" />);
  expect(screen.getByText('abcd')).toBeInTheDocument();
});

it('displays a solitary link', async () => {
  render(<LinkifiedText text="go/abcd" />);
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
});

it('displays a link at the end of the text', async () => {
  render(<LinkifiedText text="click on go/abcd" />);
  expect(screen.getByText('click on')).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
});

it('displays a link at the start of the text', async () => {
  render(<LinkifiedText text="go/abcd is great" />);
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
  expect(screen.getByText('is great')).toBeInTheDocument();
});

it('displays a link surrounded by text', async () => {
  render(<LinkifiedText text="click on go/abcd is great" />);
  expect(screen.getByText('click on')).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
  expect(screen.getByText('is great')).toBeInTheDocument();
});

it('displays two links surrounded by text', async () => {
  render(
    <LinkifiedText text="click on go/abcd then have a look at crrev.com/c/1234. Have fun!" />,
  );
  expect(screen.getByText('click on')).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
  expect(screen.getByText('then have a look at')).toBeInTheDocument();
  expect(
    screen.getByRole('link', { name: 'crrev.com/c/1234' }),
  ).toHaveAttribute('href', 'http://crrev.com/c/1234');
  expect(screen.getByText('. Have fun!')).toBeInTheDocument();
});

it('linkifies go links', async () => {
  render(<LinkifiedText text="go/abcd" />);
  expect(screen.getByRole('link', { name: 'go/abcd' })).toHaveAttribute(
    'href',
    'http://go/abcd',
  );
});

it('linkifies buganizer links', async () => {
  render(<LinkifiedText text="b/1234" />);
  expect(screen.getByRole('link', { name: 'b/1234' })).toHaveAttribute(
    'href',
    'http://b/1234',
  );
});

it('linkifies crrev links', async () => {
  render(<LinkifiedText text="crrev.com/c/1234" />);
  expect(
    screen.getByRole('link', { name: 'crrev.com/c/1234' }),
  ).toHaveAttribute('href', 'http://crrev.com/c/1234');
});

it('linkifies bare links', async () => {
  render(<LinkifiedText text="https://google.com" />);
  expect(
    screen.getByRole('link', { name: 'https://google.com' }),
  ).toHaveAttribute('href', 'https://google.com');
});
