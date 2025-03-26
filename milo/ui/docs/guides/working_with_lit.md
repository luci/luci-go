# Working with Lit

## Status Quo

Parts of LUCI UI are still using Lit. Notable areas include:

* Build steps + related
  * This should be migrated to React.
  * Migrating might be challenging while preserving:
    * Ability to cluster steps.
    * Ability to pin steps.
    * Ability to render thousands of steps without performance issue.
  * Vanilla React state management may not be good enough to implement this.
    MobX (or other state management lib) might be needed to achieve a clean
    yet performant implementation.
* Artifact pages + related
  * This should be migrated to React. Migration should be pretty
    straightforward.
* Test result tab + related
  * This should be replaced by a new view as part of the test result
    convergence project.
* Test history view
  * This should be replaced by TestHaus.

## Adding features to Lit portion of the codebase

* When changing an existing Lit component:
  * Consider migrating it to React first.
  * Update the component in place if it's too difficult.
* When adding a new component to be used in Lit:
  * Build the new component in React, then
  * Use [`@/generic_libs/components/react_lit_element`](../../src/generic_libs/components/react_lit_element/doc.md)
     to build a Lit binding for the component.
