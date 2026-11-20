package prefs

import (
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/config"
)

// The Development section's switches, as the checks say them.
const (
	DevAILabel     = "AI usage"
	DevGitHubLabel = "GitHub"
	DevGitLabLabel = "GitLab"
	DevApplyHost   = "Apply"
)

/*
buildDevelopment is the Development section's own preferences (spec 102
R12): which of the drawer's parts are drawn, how often the forges are read,
and the GitLab host and the keys where a person wants their own rather than
the ones found.

The forges are off until switched on: each is a network destination, and is
reached only once asked for (#360). An empty key or host means the one found
-- gh's for GitHub; GITLAB_TOKEN or GLAB_TOKEN and GITLAB_HOST in
~/.secrets.env, else glab's, for GitLab -- and the placeholders say so.
*/
func (w *Window) buildDevelopment(_ *shell.Shell) fyne.CanvasObject {
	get := func() config.UsageSettings { return config.Usage.Get(w.opts.Store.Config()) }
	change := func(f func(*config.UsageSettings)) {
		c := w.opts.Store.Config()
		u := config.Usage.Get(c)
		f(&u)
		w.save(config.Usage.Set(c, u))
	}
	u := get()

	ai := widget.NewCheck(DevAILabel, func(on bool) { change(func(u *config.UsageSettings) { u.HideAI = !on }) })
	ai.Checked = !u.HideAI
	github := widget.NewCheck(DevGitHubLabel, func(on bool) { change(func(u *config.UsageSettings) { u.GitHub = on }) })
	github.Checked = u.GitHub
	gitlab := widget.NewCheck(DevGitLabLabel, func(on bool) { change(func(u *config.UsageSettings) { u.GitLab = on }) })
	gitlab.Checked = u.GitLab

	// The host is taken when it is finished -- Enter, or Apply -- and not at
	// each key: a host half typed is another host, and the key would be sent
	// to it (spec 102 review, F2).
	host := widget.NewEntry()
	host.SetPlaceHolder("From GITLAB_HOST or glab")
	host.SetText(u.GitLabHost)
	applyHost := func() {
		change(func(u *config.UsageSettings) { u.GitLabHost = strings.TrimSpace(host.Text) })
	}
	host.OnSubmitted = func(string) { applyHost() }
	hostRow := container.NewBorder(nil, nil, nil, widget.NewButton(DevApplyHost, applyHost), host)

	ghKey := widget.NewPasswordEntry()
	ghKey.SetPlaceHolder("From gh")
	ghKey.SetText(u.GitHubToken)
	ghKey.OnChanged = func(v string) { change(func(u *config.UsageSettings) { u.GitHubToken = strings.TrimSpace(v) }) }

	glKey := widget.NewPasswordEntry()
	glKey.SetPlaceHolder("From ~/.secrets.env or glab")
	glKey.SetText(u.GitLabToken)
	glKey.OnChanged = func(v string) { change(func(u *config.UsageSettings) { u.GitLabToken = strings.TrimSpace(v) }) }

	form := widget.NewForm(
		widget.NewFormItem("Read forges", w.devInterval(change)),
		widget.NewFormItem("GitLab host", hostRow),
		widget.NewFormItem("GitHub token", ghKey),
		widget.NewFormItem("GitLab key", glKey),
	)
	return container.NewVBox(
		widgets.DimWrapped("What the Development drawer shows. GitHub and GitLab are two weeks of "+
			"your activity, a day per cell; each is read only while the drawer shows it, and "+
			"only once switched on here."),
		container.NewHBox(ai, github, gitlab),
		form,
	)
}

// devInterval is how often a shown forge is read. A value from the settings
// file that is not a choice is offered too, rather than changed by being seen.
func (w *Window) devInterval(change func(func(*config.UsageSettings))) fyne.CanvasObject {
	minutes := config.Usage.Get(w.opts.Store.Config()).RefreshMinutes
	if minutes <= 0 {
		minutes = config.DefaultRefreshMinutes
	}
	choices := config.RefreshChoices()
	if !slices.Contains(choices, minutes) {
		choices = append(choices, minutes)
	}
	labels := make([]string, len(choices))
	at := 0
	for i, m := range choices {
		labels[i] = fmt.Sprintf("Every %d minutes", m)
		if m == minutes {
			at = i
		}
	}
	sel := widget.NewSelect(labels, nil)
	sel.SetSelectedIndex(at)
	sel.OnChanged = func(string) {
		m := choices[sel.SelectedIndex()]
		change(func(u *config.UsageSettings) { u.RefreshMinutes = m })
	}
	return sel
}
