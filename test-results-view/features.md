# Commit Timeline
* Suggest commits range to look for culprit CL
* Squash multiple commits if all builders span across them
* point at a CL if FindIt found the culprit CL

# Error Selection
* Show heuristic on which test case is more important to look at
    * fix on the way?
    * acked by other sheriffs?

# Others
* Incorporate FindIt's (or other services', e.g. Flakiness Dashboard) test results in a builder's column, if the builds use the said builder's configuration.
    * Requires the ability to query related test results for a given builder, even though those builds may not from the same builder (they share the same builder config).

* group by tests?


# Notes
* Does FindIt look at steps or test cases when doing bisection?
* how do we handle non-test failures?
    * what are the non-test failures?
        * compilation
        * infra
