package structure_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type workflowStep struct {
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

type workflowJob struct {
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

type workflow struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

// What the job that holds write rights may use. Each of these is an Action
// that does its one thing with the job's rights; anything else in that job is
// code that would hold them too.
var publishActions = []string{
	"actions/checkout",
	"actions/download-artifact",
	"actions/attest-build-provenance",
}

// A `run` line in the publishing job names no program the repository or its
// modules supply. Reading a file as data is fine; running one is not.
var runsProjectCode = regexp.MustCompile(`(^|[\s;&|(])(go|make|bash|sh|pwsh|python3?|node)\s|(^|[;&|(])\s*"?(\./|dist/)`)

func action(uses string) string {
	name, _, _ := strings.Cut(uses, "@")
	return name
}

// releaseWorkflowFaults says how a workflow lets something other than the
// build of a release touch what the release publishes, or lets the build hold
// the release's rights.
func releaseWorkflowFaults(wf workflow) []string {
	var faults []string
	taken := map[string]bool{}
	writers := 0

	for id, job := range wf.Jobs {
		writes := len(job.Permissions) == 0
		for scope, level := range job.Permissions {
			if level == "write" && scope != "security-events" {
				writes = true
			}
		}
		if !writes {
			continue
		}
		writers++
		if id != "release" {
			faults = append(faults, fmt.Sprintf("job %s holds write rights; only release may", id))
		}
		for _, step := range job.Steps {
			if step.Uses != "" && !slices.Contains(publishActions, action(step.Uses)) {
				faults = append(faults, fmt.Sprintf("job %s holds write rights and uses %s", id, action(step.Uses)))
			}
			if action(step.Uses) == "actions/checkout" && step.With["persist-credentials"] != "false" {
				faults = append(faults, fmt.Sprintf("job %s keeps its token after checkout", id))
			}
			if action(step.Uses) == "actions/download-artifact" {
				taken[step.With["name"]] = true
			}
			for _, line := range strings.Split(strings.ReplaceAll(step.Run, "\\\n", " "), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "#") && runsProjectCode.MatchString(line) {
					faults = append(faults, fmt.Sprintf("job %s holds write rights and runs: %s", id, strings.TrimSpace(line)))
				}
			}
		}
	}
	if writers == 0 {
		faults = append(faults, "no job publishes the release")
	}
	if len(taken) == 0 {
		faults = append(faults, "the publishing job takes no artifacts")
	}

	for id, job := range wf.Jobs {
		builds := false
		for _, step := range job.Steps {
			if action(step.Uses) == "actions/upload-artifact" && taken[step.With["name"]] {
				builds = true
			}
		}
		if !builds {
			continue
		}
		for _, step := range job.Steps {
			switch action(step.Uses) {
			case "actions/checkout":
				if step.With["persist-credentials"] != "false" {
					faults = append(faults, fmt.Sprintf("job %s builds the release and keeps its token", id))
				}
			case "actions/setup-go":
				if step.With["cache"] != "false" {
					faults = append(faults, fmt.Sprintf("job %s builds the release from a cache", id))
				}
			case "actions/cache", "actions/cache/restore":
				faults = append(faults, fmt.Sprintf("job %s builds the release from a cache", id))
			}
			if regexp.MustCompile(`go test|make (test|fuzz|lint)`).MatchString(step.Run) {
				faults = append(faults, fmt.Sprintf("job %s builds the release and runs tests or the linter", id))
			}
		}
	}
	return faults
}

func TestTheReleaseIsBuiltApartFromWhatPublishesIt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "build.yml"))
	require.NoError(t, err)

	var wf workflow
	require.NoError(t, yaml.Unmarshal(data, &wf))
	assert.Empty(t, releaseWorkflowFaults(wf))
}

func TestReleaseWorkflowFaultsAreFound(t *testing.T) {
	good := `
jobs:
  build:
    permissions: {contents: read}
    steps:
      - uses: actions/checkout@abc
        with: {persist-credentials: "false"}
      - uses: actions/setup-go@abc
        with: {cache: "false"}
      - run: make release
      - uses: actions/upload-artifact@abc
        with: {name: out}
  release:
    permissions: {contents: write}
    steps:
      - uses: actions/checkout@abc
        with: {persist-credentials: "false"}
      - uses: actions/download-artifact@abc
        with: {name: out}
      - run: gh release create "$TAG" dist/*.exe
`
	cases := map[string]struct{ from, to, want string }{
		"the publishing job builds":   {`gh release create "$TAG" dist/*.exe`, `make release`, "runs: make release"},
		"the publishing job runs Go":  {`gh release create "$TAG" dist/*.exe`, `go run ./tools/x`, "runs: go run"},
		"it runs what it was handed":  {`gh release create "$TAG" dist/*.exe`, `./dist/hayami --version`, "runs: ./dist"},
		"it uses another Action":      {`actions/download-artifact@abc`, `softprops/action-gh-release@abc`, "uses softprops"},
		"the build can write":         {`permissions: {contents: read}`, `permissions: {contents: write}`, "job build holds write rights"},
		"the build inherits rights":   {`permissions: {contents: read}`, ``, "job build holds write rights"},
		"the build reads a cache":     {`{cache: "false"}`, `{cache: "true"}`, "from a cache"},
		"the build keeps its cache":   {`      - uses: actions/setup-go@abc`, "      - uses: actions/cache@abc\n      - uses: actions/setup-go@abc", "from a cache"},
		"the build runs tests":        {`make release`, `go test ./...`, "runs tests"},
		"the build lints":             {`make release`, `make lint`, "runs tests or the linter"},
		"the build keeps its token":   {"permissions: {contents: read}\n    steps:\n      - uses: actions/checkout@abc\n        with: {persist-credentials: \"false\"}", "permissions: {contents: read}\n    steps:\n      - uses: actions/checkout@abc", "builds the release and keeps its token"},
		"the publisher keeps a token": {"permissions: {contents: write}\n    steps:\n      - uses: actions/checkout@abc\n        with: {persist-credentials: \"false\"}", "permissions: {contents: write}\n    steps:\n      - uses: actions/checkout@abc", "keeps its token after checkout"},
	}

	parse := func(src string) workflow {
		var wf workflow
		require.NoError(t, yaml.Unmarshal([]byte(src), &wf))
		return wf
	}
	assert.Empty(t, releaseWorkflowFaults(parse(good)))
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, good, c.from)
			faults := releaseWorkflowFaults(parse(strings.Replace(good, c.from, c.to, 1)))
			assert.Contains(t, strings.Join(faults, "\n"), c.want)
		})
	}
}
