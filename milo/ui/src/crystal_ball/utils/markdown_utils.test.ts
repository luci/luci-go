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

import { renderAiMarkdown } from './markdown_utils';

describe('renderAiMarkdown', () => {
  it('renders formatting like headings, bold, and lists', () => {
    const md = '## Title\n\n**bold text**\n\n- item 1\n- item 2';
    const html = renderAiMarkdown(md);
    expect(html).toContain('<h2>Title</h2>');
    expect(html).toContain('<strong>bold text</strong>');
    expect(html).toContain('<li>item 1</li>');
  });

  it('escapes raw HTML tags', () => {
    const html = renderAiMarkdown(
      '<script>alert(1)</script><div id="test">text</div>',
    );
    expect(html).not.toContain('<script>');
    expect(html).not.toContain('<div');
    expect(html).toContain('&lt;script&gt;');
    expect(html).toContain('&lt;div');
  });

  it('disables markdown and raw images', () => {
    const html = renderAiMarkdown(
      '![alt text](https://attacker.example/b.png)<img src="https://attacker.example/b2.png">',
    );
    expect(html).not.toContain('<img');
  });

  it('disables markdown links, angle-bracket autolinks, and linkify', () => {
    const html = renderAiMarkdown(
      '[click me](https://attacker.example/phish), <https://attacker.example/auto>, <user@google.com>, and https://example.com',
    );
    expect(html).not.toContain('<a');
    expect(html).toContain('[click me](https://attacker.example/phish)');
    expect(html).toContain('&lt;https://attacker.example/auto&gt;');
    expect(html).toContain('&lt;user@google.com&gt;');
    expect(html).toContain('https://example.com');
  });

  it('preserves standalone reference-style lines without dropping content', () => {
    const html = renderAiMarkdown('[metric_field]: startup_latency_ms');
    expect(html).toContain('[metric_field]: startup_latency_ms');
  });

  it('renders GFM tables', () => {
    const html = renderAiMarkdown(
      '| Metric | Value |\n| --- | --- |\n| p90 | 42ms |',
    );
    expect(html).toContain('<table>');
    expect(html).toContain('<td>42ms</td>');
  });
});
