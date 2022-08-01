def test_not_supported_location_regexp_format():
    # Note: In the implementation, validation is done against generated
    # location_filters.
    # TODO(crbug/1171945): These tests may be updated to directly specify
    # location_filters once that is enabled.
    # TODO(crbug/1202952): All of these tests will be removed after Tricium
    # is folded into CV and the restrictions are removed.
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r".+/docs/.+"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_filter" of an analyzer MUST have a path_regexp that matches',
    )
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r".*_pb2.py"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_filter" of an analyzer MUST have a path_regexp that matches',
    )
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r"invalid-host.com/foo/[+]/.+\.py"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_filter" of an analyzer MUST have either both Gerrit host and project or neither',
    )
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r"https://chromium-review.googlesource.com/.*/[+]/.+\.py"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_filter" of an analyzer MUST have either both Gerrit host and project or neither',
    )
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp_exclude = [r".+\.py"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        "analyzer currently can not be used together with exclude filters",
    )

def test_watching_extensions_but_from_different_set_of_gerrit_repos():
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [
                r"https://chromium-review.googlesource.com/infra/infra/[+]/.+\.py",
                r"https://chromium-review.googlesource.com/infra/luci-py/[+]/.+\.py",
                r"https://chromium-review.googlesource.com/infra/infra/[+]/.+\.go",
                r"https://chromium-review.googlesource.com/infra/luci-go/[+]/.+\.go",
            ],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        'each extension specified in "location_regexp" or "location_filters" of an analyzer MUST have the same set of gerrit URLs',
    )

def test_with_gerrit_url_and_without_gerrit_url_together():
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [
                r".+\.py",
                r"https://chromium-review.googlesource.com/infra/luci-py/[+]/.+\.py",
            ],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_filters" of an analyzer MUST NOT mix two different formats',
    )

test_not_supported_location_regexp_format()
test_watching_extensions_but_from_different_set_of_gerrit_repos()
test_with_gerrit_url_and_without_gerrit_url_together()
