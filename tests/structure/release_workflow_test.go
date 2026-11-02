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
	Uses        string            `yaml:"uses"`
	Needs       []string          `yaml:"needs"`
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

// What that job's shell may run. A list of what is allowed, not of what is
// not: none of these runs a file from the checkout or the artifacts by itself.
// It does not look inside an argument, so `awk 'BEGIN{system(...)}'` is for
// the reviewer. This guards against a mistake; whoever can change the workflow
// can change this.
var publishCommands = []string{
	"awk", "case", "cat", "cd", "echo", "esac", "exit", "gh", "git", "ls", "sha256sum", "test",
}

var (
	singleQuoted = regexp.MustCompile(`'[^']*'`)
	doubleQuoted = regexp.MustCompile(`"(\\.|[^"\\])*"`)
	shellBreak   = regexp.MustCompile("[\n;|&()`]")
	assignment   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	buildsTests  = regexp.MustCompile(`go\s+test|make\s+(test|fuzz|lint|check)`)
	artifactOf   = regexp.MustCompile(`^\$\{\{\s*needs\.([-\w]+)\.outputs\.artifact-id\s*\}\}$`)
)

// commandsIn names the first word of each command in a shell script. Quoted
// text says nothing, unless a command substitution is inside it.
func commandsIn(script string) []string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	text := singleQuoted.ReplaceAllString(strings.Join(kept, "\n"), "")
	text = doubleQuoted.ReplaceAllStringFunc(text, func(q string) string {
		if strings.Contains(q, "$(") || strings.Contains(q, "`") {
			return strings.ReplaceAll(q, `"`, " ")
		}
		return ""
	})
	var commands []string
	for _, segment := range shellBreak.Split(text, -1) {
		words := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(segment, "{", " "), "}", " "))
		for len(words) > 0 && assignment.MatchString(words[0]) {
			words = words[1:]
		}
		if len(words) > 0 {
			commands = append(commands, words[0])
		}
	}
	return commands
}

func action(uses string) string {
	name, _, _ := strings.Cut(uses, "@")
	return strings.ToLower(name)
}

// releaseWorkflowFaults says how a workflow lets something other than the
// build of a release touch what the release publishes, or lets the build hold
// the release's rights.
func releaseWorkflowFaults(wf workflow) []string {
	var faults []string
	builders := map[string]bool{}
	writers := 0
	checksMain := false

	for id, job := range wf.Jobs {
		if job.Uses != "" {
			faults = append(faults, fmt.Sprintf("job %s is another workflow, %s", id, job.Uses))
		}
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
				match := artifactOf.FindStringSubmatch(step.With["artifact-ids"])
				if match == nil {
					faults = append(faults, fmt.Sprintf("job %s takes an artifact not by its job's ID", id))
				} else {
					builders[match[1]] = true
				}
			}
			if strings.Contains(step.Run, "merge-base --is-ancestor") && strings.Contains(step.Run, "refs/remotes/origin/main") {
				checksMain = true
			}
			for _, command := range commandsIn(step.Run) {
				if !slices.Contains(publishCommands, command) && !strings.Contains(command, "*") {
					faults = append(faults, fmt.Sprintf("job %s holds write rights and runs: %s", id, command))
				}
			}
		}
	}
	if writers == 0 {
		faults = append(faults, "no job publishes the release")
	}
	if len(builders) == 0 {
		faults = append(faults, "the publishing job takes no artifacts")
	}
	if !slices.Contains(wf.Jobs["release"].Needs, "bpf-rebuilt") {
		faults = append(faults, "the release does not wait for the BPF object to be compared")
	}
	if !checksMain {
		faults = append(faults, "nothing checks that a release is a commit on refs/remotes/origin/main")
	}

	for id := range builders {
		job, ok := wf.Jobs[id]
		if !ok {
			faults = append(faults, fmt.Sprintf("job %s is taken from and does not exist", id))
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
			case "actions/cache", "actions/cache/restore", "actions/download-artifact":
				faults = append(faults, fmt.Sprintf("job %s builds the release from a cache or another job's files", id))
			}
			if buildsTests.MatchString(step.Run) {
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
  release:
    needs: [bpf-rebuilt]
    permissions: {contents: write}
    steps:
      - uses: actions/checkout@abc
        with: {persist-credentials: "false"}
      - uses: actions/download-artifact@abc
        with: {artifact-ids: "${{ needs.build.outputs.artifact-id }}"}
      - run: git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main
      - run: gh release create "$TAG" dist/*.exe
`
	publish := `gh release create "$TAG" dist/*.exe`
	read := `permissions: {contents: read}`
	cases := map[string]struct{ from, to, want string }{
		"the publishing job builds":     {publish, `make release`, "runs: make"},
		"it runs make, last":            {publish, "echo x; make", "runs: make"},
		"it runs Go":                    {publish, `go run ./tools/x`, "runs: go"},
		"it runs what it was handed":    {publish, `./dist/hayami --version`, "runs: ./dist/hayami"},
		"it runs it under sudo":         {publish, `sudo ./dist/x`, "runs: sudo"},
		"it runs it from a pipe":        {publish, `curl x | bash`, "runs: curl"},
		"it sources a file":             {publish, `source ./x`, "runs: source"},
		"it branches on a program":      {publish, `if ./x; then echo; fi`, "runs: if"},
		"it hides in a substitution":    {publish, `echo "$(./x)"`, "runs: ./x"},
		"it uses another Action":        {`actions/download-artifact@abc`, `softprops/action-gh-release@abc`, "uses softprops"},
		"the build can write":           {read, `permissions: {contents: write}`, "job build holds write rights"},
		"the build inherits rights":     {read, ``, "job build holds write rights"},
		"the build reads a cache":       {`{cache: "false"}`, `{cache: "true"}`, "from a cache"},
		"it spells the Action oddly":    {"uses: actions/setup-go@abc\n        with: {cache: \"false\"}", "uses: Actions/Setup-Go@abc", "from a cache"},
		"the build keeps a cache":       {`      - run: make release`, "      - uses: actions/cache@abc\n      - run: make release", "from a cache"},
		"the build takes other files":   {`      - run: make release`, "      - uses: actions/download-artifact@abc\n      - run: make release", "another job's files"},
		"the build runs tests":          {`make release`, `go  test ./...`, "runs tests"},
		"the build lints":               {`make release`, `make lint`, "runs tests or the linter"},
		"it takes a file by name":       {`{artifact-ids: "${{ needs.build.outputs.artifact-id }}"}`, `{name: out}`, "not by its job's ID"},
		"nothing checks main":           {`git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main`, `echo fine`, "refs/remotes/origin/main"},
		"a short name checks main":      {`refs/remotes/origin/main`, `origin/main`, "refs/remotes/origin/main"},
		"a release skips the BPF check": {`needs: [bpf-rebuilt]`, `needs: []`, "BPF object"},
		"a job is another workflow":     {"jobs:\n", "jobs:\n  other:\n    uses: a/b/.github/workflows/c.yml@abc\n    permissions: {contents: read}\n", "is another workflow"},
		"the build keeps its token":     {read + "\n    steps:\n      - uses: actions/checkout@abc\n        with: {persist-credentials: \"false\"}", read + "\n    steps:\n      - uses: actions/checkout@abc", "builds the release and keeps its token"},
		"the publisher keeps a token":   {"permissions: {contents: write}\n    steps:\n      - uses: actions/checkout@abc\n        with: {persist-credentials: \"false\"}", "permissions: {contents: write}\n    steps:\n      - uses: actions/checkout@abc", "keeps its token after checkout"},
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
