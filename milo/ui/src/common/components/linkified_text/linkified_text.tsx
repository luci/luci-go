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

import { Fragment } from 'react';

// The patterns that will be linkified.
//
// WARNING: Be strict with what is accepted here.  If you are too liberal you
// will enable XSS attacks.
const patterns = [
  'go/[a-zA-Z0-9-_]+',
  'b/[0-9]+',
  'crrev.com/[ci]/[0-9]+',
  'https?://[a-z0-9.-]+/?[a-zA-Z0-9/.-_+]*',
];

const combinedPatterns = '(?:' + patterns.join(')|(?:') + ')';

interface LinkifiedTextProps {
  text: string | undefined;
}

/**
 * Linkified Text displays text with substrings that look links turned into
 * actual HTML links.
 *
 * There are no guarantees provided that all types of links will be handled,
 * we just try to handle the most common ones for our UIs.  In particular
 * the patterns are fairly strict to avoid XSS vulnerabilities.
 */
export const LinkifiedText = ({ text }: LinkifiedTextProps) => {
  if (!text) {
    return null;
  }
  const fragments = fragmentsFromText(text);
  return (
    <Fragment>
      {fragments.map((f, i) => (
        <Fragment key={i}>
          {f.text ? <span>{f.text}</span> : null}
          {f.link ? (
            <a href={f.link} target="_blank" rel="noreferrer">
              {f.linkText}
            </a>
          ) : null}
        </Fragment>
      ))}
    </Fragment>
  );
};

interface TextFragment {
  text: string;
  linkText?: string;
  link?: string;
}

const fragmentsFromText = (text: string): TextFragment[] => {
  const re = new RegExp(combinedPatterns, 'g');
  let lastIndex = 0;
  let result: RegExpExecArray | null = null;
  const fragments: TextFragment[] = [];

  while ((result = re.exec(text))) {
    fragments.push({
      text: text.substring(lastIndex, result.index),
      linkText: result[0],
      link: result[0].match(/https?:\/\//) ? result[0] : 'http://' + result[0],
    });
    lastIndex = result.index + result[0].length;
  }
  fragments.push({
    text: text.substring(lastIndex, text.length),
  });
  return fragments;
};
