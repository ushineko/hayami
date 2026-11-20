package forge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// ToolTimeout bounds asking gh or glab for a token.
const ToolTimeout = 5 * time.Second

// The names ~/.secrets.env is read for, and the only ones kept (spec 102 R10).
var secretNames = []string{"GITLAB_TOKEN", "GLAB_TOKEN", "GITLAB_HOST"}

/*
Creds finds the credentials and the GitLab host (spec 102 R10). Each field
is a seam a test replaces; NewCreds fills them with the real ones.

What is found is returned to the reader that asked and kept nowhere: a token
is in memory for one reading.
*/
type Creds struct {
	// Secrets is the path of the KEY=value file read for a GitLab key and
	// host: ~/.secrets.env.
	Secrets string

	// GlabConfig is glab's config file, read for its hosts.
	GlabConfig string

	// Run runs a program with fixed arguments, no shell, and returns what
	// it printed.
	Run func(ctx context.Context, name string, args ...string) (string, error)
}

// NewCreds is the desk's: ~/.secrets.env, glab's config, real programs.
func NewCreds() Creds {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	return Creds{
		Secrets:    filepath.Join(home, ".secrets.env"),
		GlabConfig: filepath.Join(cfg, "glab-cli", "config.yml"),
		Run:        run,
	}
}

// glabEnv are the variables glab answers `config get token` from before its
// own config, for any host asked about. They are taken out of glab's
// environment, so that what it gives for a host is the key it keeps for that
// host and not one that belongs somewhere else (spec 102 review, F2).
var glabEnv = []string{"GITLAB_TOKEN", "GITLAB_ACCESS_TOKEN", "OAUTH_TOKEN", "GLAB_TOKEN", "GITLAB_HOST", "GL_HOST"}

// run runs a program by name from PATH with fixed arguments and a timeout,
// and returns its standard output. Its standard error is not kept: a tool
// that fails may print what it was given.
func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ToolTimeout)
	defer cancel()
	//nolint:gosec // gh or glab by name, with this package's own fixed arguments; no shell
	cmd := exec.CommandContext(ctx, name, args...)
	if name == "glab" {
		cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
			k, _, _ := strings.Cut(kv, "=")
			return slices.Contains(glabEnv, k)
		})
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, scrub(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// scrub keeps an exit error's status and drops its standard error.
func scrub(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("exit status %d", ee.ExitCode())
	}
	return err
}

// Override is what a person typed into Preferences: a token and a host,
// either empty for "find it".
type Override struct {
	Token string
	Host  string
}

// GitHubToken is the token for github.com: the override, else gh's.
func (c Creds) GitHubToken(ctx context.Context, o Override) (string, error) {
	if t := strings.TrimSpace(o.Token); t != "" {
		return t, nil
	}
	t, err := c.Run(ctx, "gh", "auth", "token", "--hostname", "github.com")
	if err != nil || t == "" {
		return "", fmt.Errorf("%w: sign in with gh auth login, or give a token in Preferences", ErrNoCredential)
	}
	return t, nil
}

// GitLabHost is the GitLab host: the override, else GITLAB_HOST in the
// secrets file, else the first of glab's hosts glab has a token for. Empty
// when none is known.
func (c Creds) GitLabHost(ctx context.Context, o Override) string {
	if h := cleanHost(o.Host); h != "" {
		return h
	}
	if h := cleanHost(c.secrets()["GITLAB_HOST"]); h != "" {
		return h
	}
	for _, h := range c.glabHosts() {
		if t, err := c.Run(ctx, "glab", "config", "get", "token", "--host", h); err == nil && t != "" {
			return h
		}
	}
	return ""
}

/*
GitLabToken is the key for host: the override, else GITLAB_TOKEN or
GLAB_TOKEN in the secrets file, else glab's for that host.

**A key found goes only to the host it was found with** (spec 102 review,
F2): the secrets file's key only to the secrets file's GITLAB_HOST, glab's
only to the host glab keeps it for. A secrets file with a key and no host
lends its key to nobody, since sending it to whichever host was found
otherwise would hand a work key to another GitLab. A key typed into
Preferences is the person's word for the host they chose.
*/
func (c Creds) GitLabToken(ctx context.Context, host string, o Override) (string, error) {
	if t := strings.TrimSpace(o.Token); t != "" {
		return t, nil
	}
	s := c.secrets()
	if host != "" && cleanHost(s["GITLAB_HOST"]) == host {
		for _, k := range []string{"GITLAB_TOKEN", "GLAB_TOKEN"} {
			if s[k] != "" {
				return s[k], nil
			}
		}
	}
	if host != "" {
		if t, err := c.Run(ctx, "glab", "config", "get", "token", "--host", host); err == nil && t != "" {
			return t, nil
		}
	}
	return "", fmt.Errorf("%w: put GITLAB_TOKEN and GITLAB_HOST in ~/.secrets.env, sign in with glab, or give a key in Preferences", ErrNoCredential)
}

// cleanHost is a host as a person might write it, without a scheme or a
// path: "https://gitlab.example.com/" is gitlab.example.com.
func cleanHost(h string) string {
	h = strings.TrimSpace(h)
	h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	h, _, _ = strings.Cut(h, "/")
	return h
}

/*
secrets reads the secrets file for the names in secretNames and nothing else
(spec 102 R10). It is KEY=value lines, as a shell would source them: an
optional "export", optional quotes, comments. It is parsed, never run, and a
line that is not one of the names is not kept, so the file's other secrets
are not held in this program's memory past the line they were read on.
*/
func (c Creds) secrets() map[string]string {
	out := map[string]string{}
	f, err := os.Open(c.Secrets)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		k, v, ok := parseLine(sc.Text())
		if ok && slices.Contains(secretNames, k) {
			out[k] = v
		}
	}
	// A line longer than the buffer ends the read; what was read before it
	// stands, and the error says nothing a reason could use.
	_ = sc.Err()
	return out
}

// parseLine is one KEY=value line, or not ok for a comment, a blank or
// anything else.
func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	} else if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return key, value, key != ""
}

// glabHosts are the hosts in glab's config, in order of name.
func (c Creds) glabHosts() []string {
	b, err := os.ReadFile(c.GlabConfig)
	if err != nil {
		return nil
	}
	var cfg struct {
		Hosts map[string]yaml.Node `yaml:"hosts"`
	}
	if yaml.Unmarshal(b, &cfg) != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Hosts))
	for h := range cfg.Hosts {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
