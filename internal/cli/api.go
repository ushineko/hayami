package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/tcpkey"
	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/safetext"
)

/*
apiCmd is the reference client's command line (spec 113 R30): what another
program would do with the service, done from a terminal, and what the
end-to-end tests run.
*/
func apiCmd() *cobra.Command {
	f := &apiFlags{}
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Read the service with its reference client: address, status, get, watch, check, load",
		Long: "Read the service (hayami-tui serve) with its reference client.\n\n" +
			"Every response is held to the contract in api/openapi.yaml; one it does not\n" +
			"allow is an error naming the field.",
	}
	cmd.PersistentFlags().StringVar(&f.address, "address", "", "the service's socket, in place of the default address")
	cmd.PersistentFlags().BoolVar(&f.tcp, "tcp", false, "use the service's loopback TCP port, with the token from "+tcpkey.FileName+" (spec 115)")
	cmd.PersistentFlags().DurationVar(&f.timeout, "timeout", 0,
		"give up after this long (default: 1m for status and get, 15m for check, none for watch and load)")
	cmd.AddCommand(apiAddressCmd(f), apiStatusCmd(f), apiGetCmd(f), apiWatchCmd(f), apiCheckCmd(f), apiLoadCmd(f))
	return cmd
}

// apiFlags are the flags every api subcommand takes.
type apiFlags struct {
	address string
	timeout time.Duration
	tcp     bool
}

func (f *apiFlags) dial() (*client.Client, error) {
	if f.tcp {
		key, err := tcpKey()
		if err != nil {
			return nil, err
		}
		return client.NewTCP(key.Address, key.Token), nil
	}
	addr, err := apiAddress(f.address)
	if err != nil {
		return nil, err
	}
	return client.New(addr), nil
}

// tcpKey is the TCP port's key, or why there is none to use. The error never
// carries the token.
func tcpKey() (tcpkey.Key, error) {
	path, err := tcpkey.Path()
	if err != nil {
		return tcpkey.Key{}, err //nolint:wrapcheck // says what is missing
	}
	key, err := tcpkey.Read(path)
	if errors.Is(err, tcpkey.ErrNoFile) {
		return tcpkey.Key{}, errors.New("the TCP port is not set up; hayami-tui api-service tcp sets it up") //nolint:err113 // said plainly
	}
	return key, err //nolint:wrapcheck // says what is wrong with the key
}

// endpoint is what a subcommand talks to: the socket, or the TCP port with
// --tcp.
func (f *apiFlags) endpoint() (string, error) {
	if f.tcp {
		key, err := tcpKey()
		return key.Address, err
	}
	return apiAddress(f.address)
}

/*
run is a subcommand's body: its context ended by an interrupt and by the
timeout (def when --timeout is not given, none when def is 0), and what it
prints and its error made printable (B6). The service cleans what it sends;
what reaches a terminal from the socket is cleaned again here, whoever
answered on it.
*/
func (f *apiFlags) run(def time.Duration, body func(ctx context.Context, cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if limit := cmp.Or(f.timeout, def); limit > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, limit)
			defer cancel()
		}
		cmd.SetOut(cleanLines{cmd.OutOrStdout()})
		if err := body(ctx, cmd, args); err != nil {
			return errors.New(safetext.Printable(err.Error(), 0))
		}
		return nil
	}
}

func apiAddress(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	addr, err := client.Address()
	if err != nil {
		return "", fmt.Errorf("finding the service: %w", err)
	}
	return addr, nil
}

func apiAddressCmd(f *apiFlags) *cobra.Command {
	return &cobra.Command{
		Use: "address", Short: "Print the service's address", Args: cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr, err := f.endpoint()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), addr)
			return nil
		},
	}
}

func apiStatusCmd(f *apiFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Say whether the service is installed and running, where, its version and its sections",
		Long: "Say whether the service is installed and running, at which address, and what it\n" +
			"serves. What the service manager says is printed before the service is asked,\n" +
			"since asking starts one that is only listening.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: f.run(time.Minute, func(ctx context.Context, cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			addr, err := apiAddress(f.address)
			if err != nil {
				return err
			}
			state := unit.Status(ctx)
			_, _ = fmt.Fprintf(out, "address    %s\n", addr)
			_, _ = fmt.Fprintf(out, "installed  %s\n", installedLine(state))
			_, _ = fmt.Fprintf(out, "tcp        %s\n", tcpLine(state))
			c, err := f.dial()
			if err != nil {
				return err
			}
			defer c.Close()
			index, err := c.Index(ctx)
			if err != nil {
				return fmt.Errorf("the service does not answer: %w", err)
			}
			list, err := c.Sections(ctx)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s, contract %s\n", index.Service, index.Version, index.API)
			for _, s := range list.Sections {
				shown := ""
				if s.Shown {
					shown = "  shown in the panel"
				}
				_, _ = fmt.Fprintf(out, "  %-12s every %gs%s\n", s.Key, s.IntervalS, shown)
			}
			return nil
		}),
	}
}

// installedLine is what the service manager says of the service, in a line.
func installedLine(s unit.State) string {
	switch {
	case !unit.Offered():
		return "no: not set up on this system; hayami-tui serve runs it by hand"
	case !s.Installed:
		return "no: running the installer again sets it up"
	case s.Running:
		return "yes, running"
	case s.Listening:
		return "yes, listening: the next program to connect starts it"
	default:
		return unit.NotRunning
	}
}

/*
tcpLine is the TCP port in a line (spec 115 R15): set up or not, where,
whether its socket is listening, and where its token is. Never the token.
*/
func tcpLine(s unit.State) string {
	path, _ := tcpkey.Path()
	key, err := tcpkey.Read(path)
	switch {
	case errors.Is(err, tcpkey.ErrNoFile):
		return "no: not set up; hayami-tui api-service tcp sets it up"
	case err != nil:
		return "no: " + err.Error()
	case !s.Installed:
		return key.Address + ", but the service is not installed"
	case s.TCPListening:
		return key.Address + ", listening; token in " + path
	default:
		return key.Address + ", not listening; token in " + path
	}
}

func apiGetCmd(f *apiFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "get <path>",
		Short: "Print one response, /v1/sections/cooler or sections/cooler",
		Long: "Print one response, indented. The path may leave out /v1. A response the\n" +
			"contract does not document is printed, and is an error.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: f.run(time.Minute, func(ctx context.Context, cmd *cobra.Command, args []string) error {
			c, err := f.dial()
			if err != nil {
				return err
			}
			defer c.Close()
			path := args[0]
			if !strings.HasPrefix(path, "/v1") {
				path = "/v1/" + strings.TrimPrefix(path, "/")
			}
			r, err := c.Get(ctx, path)
			if err != nil {
				return err
			}
			var pretty bytes.Buffer
			if json.Indent(&pretty, r.Body, "", "  ") == nil {
				r.Body = pretty.Bytes()
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", bytes.TrimRight(r.Body, "\n"))
			if r.Status != 200 {
				return fmt.Errorf("%s answered %d", path, r.Status)
			}
			return nil
		}),
	}
}

func apiWatchCmd(f *apiFlags) *cobra.Command {
	var sections, details []string
	var full bool
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Print a stream's events as they come, until interrupted",
		Long: "Print a stream's events as they come, until interrupted: one line each, the\n" +
			"time, the event, and what it is of; with --json the reading too.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: f.run(0, func(ctx context.Context, cmd *cobra.Command, _ []string) error {
			c, err := f.dial()
			if err != nil {
				return err
			}
			defer c.Close()
			s, err := c.Events(ctx, sections, details)
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()
			return watchEvents(ctx, s, cmd.OutOrStdout(), full)
		}),
	}
	cmd.Flags().StringSliceVar(&sections, "sections", nil, "sections to watch, by key")
	cmd.Flags().StringSliceVar(&details, "details", nil, "details to watch, each key/id")
	cmd.Flags().BoolVar(&full, "json", false, "print each reading as JSON after its line")
	return cmd
}

/*
cleanLines writes each line made printable (B6): what the service sends is
cleaned by the service, and what reaches a terminal from the socket is cleaned
again here, whoever answered on it. A write is taken to be whole lines, as
the commands here write them.
*/
type cleanLines struct{ w io.Writer }

func (c cleanLines) Write(p []byte) (int, error) {
	lines := strings.Split(string(p), "\n")
	for i, l := range lines {
		lines[i] = safetext.Printable(l, 0)
	}
	if _, err := io.WriteString(c.w, strings.Join(lines, "\n")); err != nil {
		return 0, fmt.Errorf("writing: %w", err)
	}
	return len(p), nil
}

// streamEnd is what a stream's end means: nothing, when the watch was
// interrupted; the service's doing, at the end of the stream; else what broke.
func streamEnd(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return nil //nolint:nilerr // interrupted: the watch ends as asked, and that is no error
	case errors.Is(err, io.EOF):
		return errors.New("the service ended the stream")
	default:
		return err
	}
}

func watchEvents(ctx context.Context, s *client.Stream, out io.Writer, full bool) error {
	for {
		ev, err := s.Next()
		if err != nil {
			return streamEnd(ctx, err)
		}
		now := time.Now().Format(time.TimeOnly)
		var what string
		var data any
		switch ev.Name {
		case client.EventSection:
			what, data = ev.Section.Key, ev.Section
		case client.EventDetail:
			what, data = ev.Detail.Section+"/"+ev.Detail.ID, ev.Detail
		default:
			what = ev.Text
		}
		// The service cleans what it sends; what reaches a terminal is
		// cleaned here too, whoever answered on the socket.
		_, _ = fmt.Fprintf(out, "%s  %-8s %s\n", now, ev.Name, what)
		if full && data != nil {
			b, _ := json.Marshal(data)
			_, _ = fmt.Fprintf(out, "%s\n", b)
		}
	}
}

func apiCheckCmd(f *apiFlags) *cobra.Command {
	o := client.CheckOptions{}
	var quiet bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Walk every endpoint, section, detail and drill, and hold each to the contract",
		Long: "Walk the service: health, the index, every section, its details and their\n" +
			"drills, every reading at once, what is not there, and a stream. Each response\n" +
			"is held to the contract and to the full-value rules. Exits non-zero on the\n" +
			"first violation, and names it.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: f.run(15*time.Minute, func(ctx context.Context, cmd *cobra.Command, _ []string) error {
			c, err := f.dial()
			if err != nil {
				return err
			}
			defer c.Close()
			if !quiet {
				o.Out = cmd.OutOrStdout()
			}
			r, err := client.Check(ctx, c, o)
			if err != nil {
				return fmt.Errorf("check failed: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "passed: %d requests, %d sections, %d details, %d events, %d gone before asked\n",
				r.Requests, r.Sections, r.Details, r.Events, r.Gone)
			return nil
		}),
	}
	cmd.Flags().IntVar(&o.Depth, "depth", 3, "how far below a section's details to follow their drills")
	cmd.Flags().IntVar(&o.Breadth, "breadth", 3, "how many of a detail's drills to follow, 0 for every one")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only the result")
	return cmd
}

func apiLoadCmd(f *apiFlags) *cobra.Command {
	o := client.LoadOptions{}
	var normal bool
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Put many clients and streams on the service at once, and report what it did",
		Long: "Put many clients and streams on the service at once: clients loop over random\n" +
			"requests of every section, detail and drill, and streams come and go. Every\n" +
			"answer is held to the contract. Reports counts, statuses, latency and events,\n" +
			"and exits non-zero on an error. Runs at low priority unless --normal-priority.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: f.run(0, func(ctx context.Context, cmd *cobra.Command, _ []string) error {
			addr, err := apiAddress(f.address)
			if err != nil {
				return err
			}
			if !normal {
				if err := client.LowerPriority(); err != nil {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "running at the usual priority:", err)
				}
			}
			o.Address = addr
			if f.tcp {
				key, err := tcpKey()
				if err != nil {
					return err
				}
				o.Address, o.Token = key.Address, key.Token
			}
			r, err := client.Load(ctx, o)
			if err != nil {
				return err
			}
			r.Write(cmd.OutOrStdout())
			if !r.OK() {
				return errors.New("the load run saw errors")
			}
			return nil
		}),
	}
	cmd.Flags().IntVar(&o.Clients, "clients", 32, "clients looping over requests")
	cmd.Flags().IntVar(&o.Streams, "streams", 8, "event streams")
	cmd.Flags().DurationVar(&o.Duration, "duration", time.Minute, "how long to run")
	cmd.Flags().BoolVar(&normal, "normal-priority", false, "run at the usual priority, not low")
	return cmd
}
