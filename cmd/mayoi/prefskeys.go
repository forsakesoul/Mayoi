package main

import (
	"os"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/keymap"
	"mayoi/internal/settings"
	"mayoi/locales"
)

// keysPane is the keyboard shortcuts of the settings window: every
// command with its keys, which a person changes by pressing new ones.
type keysPane struct {
	w     *prefsWindow
	query string
	// byKeys has the search find the commands of the keys pressed in it,
	// found, rather than those of the text typed.
	byKeys bool
	found  []keymap.Stroke
	// recording is the command whose keys are being pressed, nil for none,
	// and strokes what was pressed of them: two for a chord.
	recording *command
	strokes   []keymap.Stroke
}

// keySource is where the keys of a command come from.
type keySource int

const (
	keyNone keySource = iota
	keyDefault
	keyUser
	keyRemoved
)

// keyRow is a command as the shortcuts show it: its keys, where they come
// from, and how many bindings of the file it has with a condition or
// arguments besides.
type keyRow struct {
	cmd    command
	keys   []keymap.Stroke
	source keySource
	more   int
}

// defaultAccel is a command's own shortcut, of its menu or of the view.
func defaultAccel(cmd command) string {
	if cmd.accel != "" {
		return cmd.accel
	}
	return platformAccel(cmd.id, viewKeys[cmd.id])
}

// rows are the commands of a's window with their keys.
func (k *keysPane) rows(a *app) []keyRow {
	km := k.w.m.keys
	var rows []keyRow
	for _, cmd := range a.commands() {
		names := commandNames(cmd.id)
		r := keyRow{cmd: cmd}
		if keys := km.Last(names...); keys != nil {
			r.keys, r.source = keys, keyUser
		} else if st, ok := keymap.ParseAccel(defaultAccel(cmd)); ok {
			r.keys, r.source = []keymap.Stroke{st}, keyDefault
			if km.Removes(names...) {
				r.keys, r.source = nil, keyRemoved
			}
		}
		if km != nil {
			for _, b := range km.Bindings {
				if !b.Removal() && (b.When != nil || len(b.Args) > 0) && slices.Contains(names, b.Command) {
					r.more++
				}
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// match reports whether the search finds the row.
func (k *keysPane) match(r keyRow) bool {
	if k.byKeys {
		return len(k.found) == 0 || len(r.keys) >= len(k.found) && slices.Equal(r.keys[:len(k.found)], k.found)
	}
	text := strings.ToLower(r.cmd.label + " " + r.cmd.id + " " + keymap.KeysString(r.keys))
	for _, word := range strings.Fields(strings.ToLower(k.query)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// keyCaps are the caps of keys as the window shows them, each stroke's
// apart for a chord.
func keyCaps(keys []keymap.Stroke) []string {
	if len(keys) == 1 {
		return []string{capsText(accelCaps(keys[0].Accel(), macKeys()))}
	}
	return chordCaps(keys)
}

func (k *keysPane) view(c *ui.Context) {
	w := k.w
	p := w.pal
	if len(w.m.wins) == 0 {
		return
	}
	a := w.m.wins[0].a
	rows := slices.DeleteFunc(k.rows(a), func(r keyRow) bool { return !k.match(r) })
	ui.Row(c.Key("keys-head")).Padding(16, 24, 12, 24).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		ui.Text(c, prefSection(prefsShortcuts)).FontSize(17).FontWeight(600).TextColor(p.text).SingleLine().Shrink(0)
		ui.Spacer(c)
		if k.byKeys {
			k.keysSearch(c)
		} else {
			ui.SearchField(c.Key("keys-search"), &k.query).Label(locales.KeysSearch()).Placeholder(locales.KeysSearch()).Width(260)
		}
		by := ui.ButtonBase(c.Key("keys-by-keys")).Width(28).Height(28).Radius(6).AlignItems(ui.Center).Justify(ui.Center).
			Label(locales.KeysSearchByKeys()).Tooltip(locales.KeysSearchByKeys()).Selected(k.byKeys)
		switch {
		case k.byKeys:
			by.Background(p.accent.Alpha(0.16))
		case by.Hovered():
			by.Background(p.hover)
		}
		by.Transition(hoverFade)
		by.Children(func() {
			col := p.muted
			if k.byKeys {
				col = p.accent
			}
			ui.Icon(c, iconKeyboard).FontSize(14).TextColor(col)
		})
		if by.Clicked() {
			k.byKeys, k.found = !k.byKeys, nil
		}
	})
	ui.Row(c.Key("keys-columns")).Padding(8, 24, 6, 24).Gap(12).AlignItems(ui.Center).Children(func() {
		head := func(s string) ui.Element {
			return ui.Text(c, strings.ToUpper(s)).FontSize(10.5).FontWeight(600).LetterSpacing(0.7).TextColor(p.faint).SingleLine()
		}
		head(locales.KeysCommand()).Grow(1).Shrink(1)
		head(locales.KeysKeys()).Width(180).Shrink(0)
		head(locales.KeysSource()).Width(88).Shrink(0)
		ui.Row(c).Width(84).Shrink(0)
	})
	ui.Scroll(c.Key("keys-rows")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, 12, 16, 12).Children(func() {
			if len(rows) == 0 {
				ui.Text(c, locales.KeysNoneFound()).FontSize(13).TextColor(p.muted).Padding(24, 12)
			}
			for _, r := range rows {
				k.row(c, r)
			}
		})
	})
}

// keysSearch is the search by keys: what is pressed in it finds the
// commands whose keys start so.
func (k *keysPane) keysSearch(c *ui.Context) {
	p := k.w.pal
	box := ui.ButtonBase(c.Key("keys-search-keys")).Width(260).Height(28).Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center).
		Background(c.Theme().Surface).BorderWidth(1, 1, 1, 1).BorderColor(p.accentDim).Label(locales.KeysPressToSearch()).Focus()
	box.HandleInput(func(ev ui.InputEvent) bool {
		switch {
		case ev.Kind == ui.InputText:
			return true
		case ev.Kind != ui.InputKeyDown:
			return false
		case ev.Key == ui.KeyEscape && ev.Mods == 0:
			k.byKeys, k.found = false, nil
		case ev.Key == ui.KeyBackspace && ev.Mods == 0:
			k.found = nil
		case keymap.KnownKey(ev.Key):
			k.found = []keymap.Stroke{{Mods: ev.Mods, Key: ev.Key}}
		}
		k.w.invalidate()
		return true
	})
	box.Children(func() {
		if len(k.found) == 0 {
			ui.Text(c, locales.KeysPressToSearch()).FontSize(12).TextColor(p.faint).SingleLine()
			return
		}
		k.caps(c, k.found)
	})
}

// caps shows keys as caps.
func (k *keysPane) caps(c *ui.Context, keys []keymap.Stroke) {
	p := k.w.pal
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		for i, cap := range keyCaps(keys) {
			ui.Text(c.Key(i), cap).FontSize(11.5).TextColor(p.text).Padding(1, 6).Radius(4).
				Background(p.hover).BorderWidth(1, 1, 1, 1).BorderColor(p.border).SingleLine()
		}
	})
}

// row is a command with its keys, and the buttons that change them.
func (k *keysPane) row(c *ui.Context, r keyRow) {
	p := k.w.pal
	row := ui.ButtonBase(c.Key("keys-row-"+r.cmd.id)).Padding(6, 12).Gap(12).Radius(6).AlignItems(ui.Center).Label(r.cmd.label)
	if row.Hovered() {
		row.Background(p.hover)
	}
	hovered := row.Hovered()
	row.Children(func() {
		ui.Column(c).Gap(1).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, r.cmd.label).FontSize(12.5).TextColor(p.text).SingleLine()
			ui.Text(c, r.cmd.id).Font(k.w.monoFont()).FontSize(10.5).TextColor(p.faint).SingleLine()
		})
		ui.Row(c).Width(180).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			if len(r.keys) > 0 {
				k.caps(c, r.keys)
			} else {
				ui.Text(c, "—").FontSize(12).TextColor(p.faint)
			}
			if r.more > 0 {
				ui.Text(c, locales.KeysMore(r.more)).FontSize(10.5).TextColor(p.faint).SingleLine().
					Tooltip(locales.KeysMoreTip())
			}
		})
		source, col := "", p.faint
		switch r.source {
		case keyDefault:
			source = locales.KeysDefault()
		case keyUser:
			source, col = locales.KeysUser(), p.accent
		case keyRemoved:
			source, col = locales.KeysRemoved(), p.modifiedFg
		}
		ui.Text(c, source).FontSize(11.5).TextColor(col).Width(88).Shrink(0).SingleLine()
		ui.Row(c).Width(84).Shrink(0).Gap(2).Justify(ui.End).Children(func() {
			if !hovered {
				return
			}
			k.action(c, "change:"+r.cmd.id, iconPencil, locales.KeysChange(), true, func() { k.record(r.cmd) })
			k.action(c, "remove:"+r.cmd.id, iconMinus, locales.KeysRemove(), len(r.keys) > 0, func() { k.removeKeys(r.cmd) })
			k.action(c, "reset:"+r.cmd.id, iconUndo, locales.KeysReset(), r.source == keyUser || r.source == keyRemoved, func() { k.resetKeys(r.cmd) })
		})
	})
	if row.Clicked() {
		k.record(r.cmd)
	}
}

// action is a button of a row of the shortcuts.
func (k *keysPane) action(c *ui.Context, key string, icon *ui.SVG, label string, on bool, fn func()) {
	p := k.w.pal
	b := ui.ButtonBase(c.Key("keys-" + key)).Width(24).Height(24).Radius(5).AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(label).Disabled(!on)
	if b.Hovered() && on {
		b.Background(p.surfaceHover)
	}
	b.Children(func() {
		col := p.muted
		if !on {
			col = p.faint.Alpha(0.5)
		}
		ui.Icon(c, icon).FontSize(12.5).TextColor(col)
	})
	if b.Clicked() && on {
		fn()
	}
}

// record starts taking the keys pressed as the new keys of cmd.
func (k *keysPane) record(cmd command) {
	k.recording, k.strokes = &cmd, nil
	k.w.invalidate()
}

// stroke takes a key pressed for the command recording: a third starts
// the keys again.
func (k *keysPane) stroke(s keymap.Stroke) {
	if len(k.strokes) >= 2 {
		k.strokes = nil
	}
	k.strokes = append(k.strokes, s)
	k.w.invalidate()
}

// recordAccel takes the shortcut of command id of the menus as a key
// pressed while the keys are recorded, as the menus of macOS take their
// shortcuts before the window: it reports whether it took it.
func (k *keysPane) recordAccel(id string) bool {
	if k.recording == nil || len(k.w.m.wins) == 0 {
		return false
	}
	for _, cmd := range k.w.m.wins[0].a.commands() {
		if cmd.id == id {
			if st, ok := keymap.ParseAccel(k.w.m.wins[0].a.menuAccel(cmd)); ok {
				k.stroke(st)
				return true
			}
		}
	}
	return false
}

// conflicts are the other commands whose keys are those pressed.
func (k *keysPane) conflicts() []string {
	if k.recording == nil || len(k.strokes) == 0 || len(k.w.m.wins) == 0 {
		return nil
	}
	var out []string
	for _, r := range k.rows(k.w.m.wins[0].a) {
		if r.cmd.id != k.recording.id && slices.Equal(r.keys, k.strokes) {
			out = append(out, r.cmd.label)
		}
	}
	return out
}

// recordView is the dialog that takes the keys pressed for a command.
func (k *keysPane) recordView(c *ui.Context) {
	cmd := k.recording
	if cmd == nil {
		return
	}
	p := k.w.pal
	done := func(save bool) {
		if save && len(k.strokes) > 0 {
			k.setKeys(*cmd, k.strokes)
		}
		k.recording, k.strokes = nil, nil
	}
	ui.Overlay(c, func() {
		back := ui.Column(c.Key("keys-record-back")).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			AlignItems(ui.Center).Justify(ui.Center).Background(ui.RGBA(0, 0, 0, 0.25)).Modal()
		back.Children(func() {
			box := ui.Column(c.Key("keys-record")).Width(420).MaxWidthPercent(92).Padding(20).Gap(14).Radius(12).
				Background(c.Theme().Surface).Border(1, p.border).Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.3)).
				Role(ui.RoleDialog).Label(locales.KeysRecordTitle(cmd.label)).Focusable().Focus().FocusRing(false)
			box.HandleInput(func(ev ui.InputEvent) bool {
				switch {
				case ev.Kind == ui.InputText:
					return true
				case ev.Kind != ui.InputKeyDown:
					return false
				case ev.Key == ui.KeyEscape && ev.Mods == 0:
					done(false)
				case ev.Key == ui.KeyEnter && ev.Mods == 0:
					done(true)
				case ev.Key == ui.KeyBackspace && ev.Mods == 0:
					k.strokes = nil
				case keymap.KnownKey(ev.Key):
					k.stroke(keymap.Stroke{Mods: ev.Mods, Key: ev.Key})
				}
				k.w.invalidate()
				return true
			})
			box.Children(func() {
				ui.Text(c, locales.KeysRecordTitle(cmd.label)).FontSize(14).FontWeight(600).TextColor(p.text)
				ui.Row(c.Key("keys-record-keys")).Height(52).Radius(8).AlignItems(ui.Center).Justify(ui.Center).
					Background(p.hover).BorderWidth(1, 1, 1, 1).BorderColor(p.accentDim).Children(func() {
					if len(k.strokes) == 0 {
						ui.Text(c, locales.KeysPress()).FontSize(13).TextColor(p.faint)
						return
					}
					k.caps(c, k.strokes)
				})
				ui.Text(c, locales.KeysRecordHint()).FontSize(11.5).TextColor(p.muted)
				if others := k.conflicts(); len(others) > 0 {
					ui.Row(c.Key("keys-record-conflict")).Gap(8).Padding(8, 10).Radius(6).Background(p.modified.Alpha(0.12)).
						AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconTriangleAlert).FontSize(13).TextColor(p.modifiedFg).Shrink(0)
						ui.Text(c, locales.KeysConflict(strings.Join(others, ", "))).FontSize(11.5).TextColor(p.text).Grow(1).Shrink(1)
					})
				}
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c.Key("keys-record-cancel"), locales.ButtonCancel()).Clicked() {
						done(false)
					}
					if ui.PrimaryButton(c.Key("keys-record-save"), locales.KeysSave()).Disabled(len(k.strokes) == 0).Clicked() {
						done(true)
					}
				})
			})
		})
	})
}

// unconditional reports whether an entry of the file binds, or takes
// away, the keys of one of names without a condition or arguments.
func unconditional(e keymap.Entry, names []string) bool {
	return e.When == "" && len(e.Args) == 0 && slices.Contains(names, strings.TrimPrefix(e.Command, "-"))
}

// defaultRemoval is the entry that takes cmd's own shortcut away, false
// when it has none.
func defaultRemoval(cmd command) (keymap.Entry, bool) {
	st, ok := keymap.ParseAccel(defaultAccel(cmd))
	if !ok {
		return keymap.Entry{}, false
	}
	return keymap.Entry{Key: st.String(), Command: "-" + cmd.id}, true
}

// setKeys gives cmd the keys in keybindings.json, in place of those it
// had, its own shortcut taken away.
func (k *keysPane) setKeys(cmd command, keys []keymap.Stroke) {
	names := commandNames(cmd.id)
	var add []keymap.Entry
	if rm, ok := defaultRemoval(cmd); ok && rm.Key != keymap.KeysString(keys) {
		add = append(add, rm)
	}
	add = append(add, keymap.Entry{Key: keymap.KeysString(keys), Command: cmd.id})
	k.edit(func(e keymap.Entry) bool { return unconditional(e, names) }, add)
}

// removeKeys takes cmd's keys away.
func (k *keysPane) removeKeys(cmd command) {
	names := commandNames(cmd.id)
	var add []keymap.Entry
	if rm, ok := defaultRemoval(cmd); ok {
		add = append(add, rm)
	}
	k.edit(func(e keymap.Entry) bool { return unconditional(e, names) }, add)
}

// resetKeys gives cmd its own shortcut back.
func (k *keysPane) resetKeys(cmd command) {
	names := commandNames(cmd.id)
	k.edit(func(e keymap.Entry) bool { return unconditional(e, names) }, nil)
}

// edit changes keybindings.json, as keymap.Edit does, and applies it at
// once.
func (k *keysPane) edit(drop func(keymap.Entry) bool, add []keymap.Entry) {
	m := k.w.m
	path := keymap.Path(m.s.Path())
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		k.w.errors = append(k.w.errors, err.Error())
		return
	}
	out, err := keymap.Edit(data, drop, add)
	if err != nil {
		k.w.errors = append(k.w.errors, locales.KeysCouldNotEdit(err.Error()))
		return
	}
	if err := settings.WriteFile(path, out); err != nil {
		k.w.errors = append(k.w.errors, locales.FailSave(path, err.Error()))
		return
	}
	km, err := keymap.Parse(out)
	if len(m.wins) == 0 {
		if err == nil {
			m.keys = km
		}
	} else {
		m.reloadKeys(km, err)
	}
	k.w.invalidate()
}
