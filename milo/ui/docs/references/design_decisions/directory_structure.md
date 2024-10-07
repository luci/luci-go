# [Src Directory](../../src) Structure
## Goals
The directory structure is designed to achieve the following goals:
 * Fit to host all LUCI UI projects.
 * Discovering/locating existing modules should be straightforward.
 * Deciding where a new module should be placed should not be confusing.
 * Managing (first party) modules dependencies should be straightforward, which
   requires
   * making the dependency relationship between modules more obvious, and
   * making it harder to accidentally introduce circular dependencies.
 * Support different levels of encapsulation, which includes the abilities to
   * limit the surface of a module, and
   * enforce module-level invariants.

## Rules
To achieve those goals, the [./src](../../src) directory are generally
structured with the following rules:
 * Modules are grouped into packages. Packages are all located at the top level
   directory in the ./src directory. It can be one of the followings:
   * A business domain package (e.g. [@/build](../../src/build),
     [@/bisection](../../src/bisection)).
     * The contained modules are specific to the business domain.
     * The business domain typically matches a top-level navigation/feature
       area.
     * A domain package may import from another domain package
       * This is supported so natural dependencies between domains can be
         expressed (e.g. builds depends on test results).
       * The usage should still be limited. Consider lifting the shared module
         to the @/common package.
       * Circular dependencies between domain packages must be avoided.
     * Grouping modules by business domains makes enforcing encapsulation/
       isolation easier.
     * Placing modules under a package named after their domain helps
       discovering/locating modules as the project grows larger.
   * The [@/common](../../src/common) package.
     * The contained modules can have business/application logic.
     * The contained modules must not make assumptions on the business domains
       it's used in.
     * The contained modules should be reusable across business domains.
     * Must not import from domain packages.
     * This helps capturing modules that cross domains.
     * The name gives a clear signal that the modules should stay reusable.
   * The [@/generic_libs](../../src/generic_libs) package.
     * The contained modules must not contain any business logic.
     * The contained modules should be highly generic and reusable (akin to a
       published module).
     * The contained modules (excluding unit tests) must not depend on any first
       party module outside of [@/generic_libs](../../src/generic_libs).
     * Comparing to @/common, [@/generic_libs](../../src/generic_libs) must not
       contain business/application logic.
     * The name gives a clear signal that the modules should stay generic.
     * Separating from @/common makes it harder to accidentally
       add business logic to a generic module.
   * The [@/core](../../src/core) package.
     * The contained modules are for core functionality such as login or
       the landing page.
   * The [@/testing_tools](../../src/testing_tools) package.
     * Contains utilities for writing unit tests.
     * Can import from other packages or be imported to other packages.
     * Must only be used in unit tests.
     * Other directories may have their own `./testing_tools` subdirectory to
       contain testing tools specific to those domains.
     * It helps separating test utilities from production code.
 * Modules are usually further divided into groups by their functional category
   (e.g. `./components/*`, `./hooks/*`, `./tools/*`).
   * The purpose is to make locating/discovering existing modules easier as the
     module list grows larger.
   * The divide is purely aesthetical.
     * There's no logical boundary between those groups and the division does
       not signal or enforce encapsulation.
     * It's perfectly fine to have "circular imports" between groups since they
       are merely aesthetical groups.
     * Circular dependencies between the actual underlying modules should still
       be avoided.
   * This rule is enforced loosely.
     * Modules belong to multiple functional categories can simply pick a group
       with the best fit (e.g. a module that exports React components, hooks and
       utility functions may simply be placed under `./components/`).
     * Modules that don't fit any of the categories may be placed directly in
       the parent directory or in a catch all group (e.g. `./tools/`). This
       should be used sparingly.
 * Modules may declare entry files (e.g. `index.ts`) that reexport symbols.
   Symbols reexported by the entry file are considered the public surface of the
   module, while other symbols are considered internal to the module (i.e. no
   deep imports when there's an entry file).
   * This helps reducing the public surface of a module. Makes it easier to
     implement encapsulation and enforce invariants.
 * Modules can themselves have different internal structures to implement
   different layers of encapsulation.
 * Anything related to a set of contexts (e.g. contexts, context-related hooks,
   providers) should be placed under a single directory (e.g.
   `my-module/context/`). Providers and hooks should be placed in different
   files under that directory so
   [React fast refresh](https://reactnative.dev/docs/fast-refresh#how-it-works)
   works.

Note: At the moment (2023-09-14), some packages are in an inconsistent state.
Some modules should be moved to other packages. Notable items include but not
limited to
 * Some modules in @/common should be moved to business domain packages.
 * [@/bisection](../../src/bisection) and other recently merged in projects
   should have common modules lifted to @/common.

### Graph illustration of the package relationships:
```ascii
@/core                   ─┬─> @/build                        ─┬─> @/common           ─┬─> @/generic_libs
  ├─■ ./pages             │     ├─■ ./pages                   │     ├─■ ./components  │     ├─■ ./components
  ├─■ ...other groups...  │     ├─■ ./components              │     ├─■ ./hooks       │     ├─■ ./hooks
  ├─■ ...entry files...   │     ├─■ ./hooks                   │     ├─■ ./tools       │     ├─■ ./tools
  └─■ ...                 │     ├─■ ./tools                   │     └─■ ...           │     └─■ ...
                          │     └─■ ...                       │                       │
                          │                                   │                       ├─> ...third party libs...
                          ├─> @/bisection                    ─┤                       │
                          │     ├─■ ./pages                   │                       │
                          │     ├─■ ./components              │                       │
                          │     ├─■ ./hooks                   │                       │
                          │     ├─■ ./context                 │                       │
                          │     ├─■ ./tools                   │                       │
                          │     └─■ ...                       │                       │
                          │                                   │                       │
                          ├─> @/analysis                     ─┤                       │
                          │     └─■ ...                       │                       │
                          │                                   │                       │
                          ├─> @/...other business domains... ─┤                       │
                          │     └─■ ...                       │                       │
                          │                                   │                       │
                          └─> ────────────────────────────────┴─> ────────────────────┘

A ─> B: A depends on B.
A ─■ B: A contains B.
```
