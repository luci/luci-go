# Documentation for package.json
Since package.json does not support comments, we place the documentation for
package.json in this file.

## "Un-upgradable" dependencies
The following dependencies should not be upgraded to the next major version.

### codemirror@5 + related
codemirror@6 no longer supports setting `viewportMargin`, which is required to
support searching content hidden behind a scrollbar. `viewportMargin` is
currently relied on in
`src/common/components/property_viewer/property_viewer.tsx` (`5000`) and
`src/test_verdict/legacy/test_history_page/test_properties_entry.tsx`
(`Infinity`).

Additionally, as documented in
`src/generic_libs/components/code_mirror_editor.tsx`, neither
`@uiw/react-codemirror@3` nor `@uiw/react-codemirror@4` lets us attach
fold/unfold listeners before the content is rendered, and `@3` produces
react-dom validation errors on the React version we use.

To upgrade to codemirror@6, we need to implement both a custom search box and
custom fold-event handling for the content rendered in the codemirror editor.

### eslint@9 + related
eslint@10 is blocked by plugins that still cap their peer range at eslint@9:
`eslint-plugin-jsx-a11y`, `eslint-plugin-import` and `eslint-plugin-react`.

Note that npm already reports eslint@9 itself as end-of-life, so this should be
revisited whenever those three plugins publish eslint@10 support.

(This section previously described eslint@8 and the flat config migration. That
migration is done -- the repo is on eslint@9 with a flat `eslint.config.js`.)

### lit@2 + related
Lit is deprecated in favor of React in LUCI UI. There's no point investing the
effort to migrate to lit@3.

Once we migrated the remaining Lit components to React, we should remove lit
from the dependency list, or upgrade it if we decide to keep Lit to support
custom artifact tags in summary_html.

### mobx@6 + related
mobx@7 is a peer dependency deadlock, not a policy choice:
* `@adobe/lit-mobx` peers `mobx@^5 || ^6`, and
* `mobx-utils` peers `mobx@^6` (there is no mobx@7-compatible release),

while `mobx-react-lite@5` and `mobx-state-tree@8` both require `mobx@^7`.

`@adobe/lit-mobx` goes away with the Lit retirement, but `mobx-utils` does not:
several of its usages are in non-Lit code. We only use 4 symbols from it
(`fromPromise`, `IPromiseBasedObservable`, `PENDING`, `REJECTED`), all funnelled
through `src/generic_libs/tools/mobx_utils/mobx_utils.ts`, so vendoring those is
the intended way out.

mobx@7 additionally requires Stage-3 `accessor` decorators, whereas we use
legacy decorators (`tsconfig.json` `experimentalDecorators`, and
`babel.config.json` `@babel/plugin-proposal-decorators` `version: "legacy"`).
That same legacy decorator setup also blocks `@babel/core@8` and
`@vitejs/plugin-react@6`.

### typescript@5 + related
typescript@7 (the native compiler port) is blocked by `typescript-eslint@8`,
which peers `typescript >=4.8.4 <6.1.0`. TypeScript 6.0 is therefore allowed,
but 7.x is not.

### @babel/core@7 + related
`@babel/core@8` is blocked by `ts-jest@29` (peerOptional `<8`) and by
`babel-preset-current-node-syntax`, pulled in via `babel-jest`.

## Exact-pinned dependencies
These are pinned to an exact version (no `^`) to work around a specific
problem. Each should be unpinned once its blocker is resolved.

Note that a caret range is NOT enough to hold a version back. Regenerating
`package-lock.json` floats every caret range to its newest match, so `^3.7.1`
will still resolve to 3.7.5.

Also note that `npm-check-updates` selects the highest published version number,
which is not always the version tagged `latest`. Before accepting an ncu bump,
it is worth checking `npm view <pkg> dist-tags`.

### echarts-for-react
Pinned to 3.0.6, which is the version tagged `latest`. 3.0.7 exists and is a
higher version number, but npm marks it as published in error and the `latest`
dist-tag was never moved to it.

3.0.7 also declares:

```json
"optionalDependencies": {
  "@antv/setup": "github:antvis/G2#7cb42f57561c321ecb09b4552802ae0ac55b3a7a"
}
```

which is a git dependency on an arbitrary commit in an unrelated charting
project. A git dependency is fetched straight from GitHub and therefore bypasses
the npm mirror entirely, so this must not be allowed into the tree.

An exact pin is required here: `^3.0.6` still resolves to 3.0.7.

Unpin if and when a 3.0.8 or later is published and tagged `latest`.


### prettier
prettier is otherwise an undeclared, transitive dependency of
`eslint-plugin-prettier`, which means a lockfile regeneration silently upgrades
the formatter and reformats the entire repository. That happened with
3.6.2 -> 3.9.6, which produced several hundred `prettier/prettier` errors in a
CL that was only meant to bump other dependencies.

It is declared here explicitly so that a formatter upgrade is always a
deliberate change with its own reformatting CL.

Note there is no `.prettierrc`; the only option we set is `singleQuote: true`,
inline in the `prettier/prettier` rule in `eslint.config.js`. Reproduce a
formatting result with `npx prettier@<version> --single-quote --check <file>`.

### @types/luxon
`@types/luxon@3.7.2` made `DateTime` invalid-aware, so `DateTime.invalid(...)`
returns `DateTime<true> | DateTime<false>`. `@mui/x-date-pickers@7` declares
`MuiPickersAdapter<DateTime<true>, string>` and rejects that union.

Unpin when `@mui/x-date-pickers` is upgraded to v8 or later.

### @tanstack/query-async-storage-persister
Held at the same version as `@tanstack/react-query` and
`@tanstack/react-query-persist-client`, which are also pinned.
`@tanstack/react-query@5.102` removed `promise` from `QueryObserver*Result`,
which breaks the mocks in `src/crystal_ball/tests/query_mocks.ts`. Those mocks
intentionally set `promise` with a `.catch()` attached so that unhandled
rejections do not crash Jest workers, so this needs a real fix rather than
deleting the field.

Unpin the `@tanstack/*` group together once the mocks are reworked.
