# Documentation for package.json
Since package.json does not support comments, we place the documentation for
package.json in this file.

## "Un-upgradable" dependencies
The following dependencies should not be upgraded to the next major version.

### codemirror@5 + related
codemirror@6 no longer supports setting `viewportMargin`, which is required to
support searching content hidden behind a scrollbar.

To upgrade to codemirror@6, we should implement custom search box for the
content rendered in the codemirror editor.

### eslint@8 + related
eslint@9 uses the [flat eslint config](1). The flat eslint config is not yet
supported by many eslint plugins (e.g. [eslint-plugin-react-hooks](2)). Most
plugins' documentation are still written for the old config format as of
2024-09-30.

[1]: https://eslint.org/docs/latest/use/configure/configuration-files
[2]: https://github.com/facebook/react/issues/28313.

### lit@2 + related
Lit is deprecated in favor of React in LUCI UI. There's no point investing the
effort to migrate to lit@3.

Once we migrated the remaining Lit components to React, we should remove lit
from the dependency list, or upgrade it if we decide to keep Lit to support
custom artifact tags in summary_html.
