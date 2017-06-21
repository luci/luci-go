package buildbot

// We put this here because _test.go files are sometimes not built.
var TestCases = []struct {
	Builder string
	Build   int
}{
	{"CrWinGoma", 30608},
	{"chromium_presubmit", 426944},
	{"newline", 1234},
	{"gerritCL", 1234},
	{"win_chromium_rel_ng", 246309},
}
