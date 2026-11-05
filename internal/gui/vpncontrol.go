package gui

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
A VPN's rows that do something (spec 085 R17): connect, and with two presses
disconnect and bounce. They are rows and not buttons because a drawer's buttons are
in its heading and belong to the whole of it, and there is a VPN per part.

The second press answers a question drawn where the first landed, as the kill's does,
so the pointer is already over the answer. The question lapses by itself: a row has
nowhere to put a cancel, and a confirmation left standing would turn a click made
minutes later into a tunnel going down.
*/

// vpnConfirmLapse is how long a row waits for its second press.
const vpnConfirmLapse = 5 * time.Second

// vpnWorking is what a row says while its press is under way.
var vpnWorking = map[string]string{
	view.VPNConnect: "connecting", view.VPNDisconnect: "disconnecting", view.VPNBounce: "bouncing",
	view.VPNReenable: "re-enabling",
}

// vpnSignInWaiting is what a row says while the browser has the sign-in page.
const vpnSignInWaiting = "sign in in your browser"

// vpnNoBrowser is what a row says when the sign-in page would not open.
const vpnNoBrowser = "sign-in page did not open"

// vpnRun is a press under way.
type vpnRun struct {
	action string
	cancel context.CancelFunc
}

// vpnPresses is what the drawer knows of the presses on a VPN's rows: the question
// standing, what is under way, and the last word on each connection.
type vpnPresses struct {
	asking  string
	askedAt time.Time
	running map[string]*vpnRun
	said    map[string]view.DetailLine
}

// pressVPN does what a press on a VPN's row asks. One press at a time on a
// connection: the same one again is ignored, since a second connect would close the
// session waiting at its sign-in page, and another replaces it, so that a person can
// disconnect a connect that is taking too long.
func (p *Panel) pressVPN(ask, action, name string) {
	d := p.drawer
	src, ok := p.source(d.section)
	if !ok {
		return
	}
	control, ok := src.(panel.VPNController)
	if !ok {
		return
	}
	onePress := action == view.VPNConnect || action == view.VPNReenable
	if !onePress && !p.confirmed(ask) {
		d.vpn.asking, d.vpn.askedAt = ask, time.Now()
		p.renderDrawer()
		return
	}
	d.vpn.asking = ""
	running := d.vpn.running[name]
	if running != nil && running.action == action {
		return
	}
	if running != nil {
		running.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &vpnRun{action: action, cancel: cancel}
	d.vpn.running[name] = run
	d.vpn.said[name] = view.VPNSaid(vpnWorking[action], false)
	p.renderDrawer()

	say := func(line view.DetailLine, last bool) {
		p.do(func() {
			if d.vpn.running[name] != run {
				return // replaced by a later press, which has the last word
			}
			d.vpn.said[name] = line
			if last {
				delete(d.vpn.running, name)
				if line.Value == "" {
					delete(d.vpn.said, name)
				}
			}
			p.renderDrawer()
		})
	}
	signIn := func(u *url.URL) {
		if err := d.openURL(u); err != nil {
			say(view.VPNSaid(vpnNoBrowser, true), false)
			return
		}
		say(view.VPNSaid(vpnSignInWaiting, false), false)
	}
	go func() {
		defer cancel()
		if err := control.ControlVPN(ctx, action, name, signIn); err != nil {
			say(view.VPNSaid(err.Error(), true), true)
			return
		}
		say(view.DetailLine{}, true)
	}()
}

// confirmed reports whether this press answers the question standing on its row.
func (p *Panel) confirmed(ask string) bool {
	v := &p.drawer.vpn
	return v.asking == ask && time.Since(v.askedAt) < vpnConfirmLapse
}

// withVPNControls draws each VPN's rows and last word over its lines. They are the
// desktop panel's: the section offers them, and a terminal has no drawer to press
// them in (spec 085 R20). A row to press starts with an arrow, because in every other
// way it looks like the readings under it.
func (p *Panel) withVPNControls(d view.Detail) view.Detail {
	if d.ID != view.DetailVPN {
		return d
	}
	marker := p.words().press
	parts := make([]view.Detail, len(d.Parts))
	for i, part := range d.Parts {
		var lines []view.DetailLine
		for _, c := range part.Controls {
			if p.confirmed(c.Open) {
				c.Value, c.Status = confirmLabel, view.Warn
			}
			c.Name = marker + c.Name
			lines = append(lines, c)
		}
		name := strings.TrimPrefix(part.ID, view.DetailVPN+"/")
		if said, ok := p.drawer.vpn.said[name]; ok {
			lines = append(lines, said)
		}
		part.Lines = append(lines, part.Lines...)
		parts[i] = part
	}
	d.Parts = parts
	return d
}
