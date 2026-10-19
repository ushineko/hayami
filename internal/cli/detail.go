package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// DetailSettle is how long `detail` waits between its two readings. A rate
// is a difference, so a list of rates read once has nothing in it; the
// second reading is the one printed. Three seconds, because Windows hands
// over the first events of a network trace about two seconds after it starts.
const DetailSettle = 3 * time.Second

/*
Detail reads one detail and writes it, as text or as JSON (spec 060 R21).

subject is "<section>" or "<section>/<id>": a section alone is its first
detail. The section is polled first, because a detail is of a reading: the
cooler has no list for a card it has not found. The detail is then read
twice, settle apart, and the second is written.
*/
func Detail(ctx context.Context, w io.Writer, sources []panel.Source, subject string, asJSON bool, settle time.Duration) error {
	section, id, _ := strings.Cut(subject, "/")
	var src panel.Source
	for _, s := range sources {
		if s.Key() == section {
			src = s
		}
	}
	if src == nil {
		return fmt.Errorf("no section named %q; there is %s", section, strings.Join(panel.Keys(), ", "))
	}
	// The poll's own failure is the section's to report; the detail says
	// what it could not read.
	_, _ = src.Poll(ctx)

	d, ok := src.(panel.Detailer)
	var offered []string
	if ok {
		for _, info := range d.Details() {
			offered = append(offered, info.ID)
		}
	}
	if len(offered) == 0 {
		return fmt.Errorf("the %s section has no detail to show", section)
	}
	if id == "" {
		id = offered[0]
	}
	if !panel.Offers(src, id) {
		return fmt.Errorf("the %s section has no detail %q; there is %s", section, id, strings.Join(offered, ", "))
	}

	defer d.CloseDetail(id)
	detail, _ := d.PollDetail(ctx, id)
	if len(detail.Reasons) == 0 {
		select {
		case <-ctx.Done():
		case <-time.After(settle):
		}
		// The section is polled again as well as the detail. A section's
		// rate is a difference too, and a detail that is a share of one --
		// the bandwidth note, the peer breakdown's remainder -- has nothing
		// to be a share of until the second reading.
		_, _ = src.Poll(ctx)
		detail, _ = d.PollDetail(ctx, id)
	}

	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(detail); err != nil {
			return fmt.Errorf("writing the detail: %w", err)
		}
		return nil
	}
	if _, err := io.WriteString(w, detail.Text()); err != nil {
		return fmt.Errorf("writing the detail: %w", err)
	}
	return nil
}

// details reads each detail a source offers, once, for the readings and for
// doctor: what each list is and whether it could be read. A list of rates
// read once has no lines yet, which is the honest answer, as it is for a
// section's rates.
func details(ctx context.Context, s panel.Source) map[string]view.Detail {
	d, ok := s.(panel.Detailer)
	if !ok {
		return nil
	}
	out := map[string]view.Detail{}
	for _, info := range d.Details() {
		detail, _ := d.PollDetail(ctx, info.ID)
		d.CloseDetail(info.ID)
		out[info.ID] = detail
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// detailCmd prints one detail and exits.
func detailCmd(f *flags) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "detail <section>[/<id>]",
		Short: "Print what is behind a reading and exit",
		Long: "Print what is behind a reading and exit.\n\n" +
			"What the window shows in its drawer when a card is clicked: the processes\n" +
			"behind the processor's load, the memory in use, the graphics card or the\n" +
			"network, and every account's limits behind the usage meters. A section\n" +
			"alone is the first detail it offers.\n\n" +
			"  hayami-tui detail cooler/cpu\n" +
			"  hayami-tui detail cooler/memory\n" +
			"  hayami-tui detail bandwidth\n" +
			"  hayami-tui detail usage\n" +
			"  hayami-tui detail \"usage/CC max\" --json\n\n" +
			"A process is listed by its name and nothing else. The pane itself has no\n" +
			"detail view.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, _, err := f.resolve()
			if err != nil {
				return err
			}
			warn(cmd, opts)
			return Detail(cmd.Context(), cmd.OutOrStdout(), opts.AllSources(nil), args[0], asJSON, DetailSettle)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the detail as JSON")
	return cmd
}
