def test_not_supported_location_regxp_fomat():
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r".+/docs/.+"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_regexp" of an analyzer MUST either be in the format of',
    )
    assert.fails(
        lambda: luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [r"chromium-review/.+/.+\.py"],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        '"location_regexp" of an analyzer MUST either be in the format of',
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
        'each extension specified in "location_regexp" of an analyzer MUST have the same set of gerrit URLs',
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
        '"location_regexp" of an analyzer MUST NOT mix two different formats',
    )

test_not_supported_location_regxp_fomat()
test_watching_extensions_but_from_different_set_of_gerrit_repos()
test_with_gerrit_url_and_without_gerrit_url_together()

