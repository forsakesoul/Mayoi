package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/git"
	"mayoi/locales"
)

// worktreeFixture is the harness's repository with a linked worktree
// named side, on a branch of the same name, and the box of the worktrees
// open on it.
func worktreeFixture(t *testing.T) (*harness, func(args ...string) string, string) {
	t.Helper()
	hs := newHarness(t, true)
	run := hs.gitRepo()
	side := filepath.Join(t.TempDir(), "side")
	run("worktree", "add", "-b", "side", side)
	return hs, run, side
}

// openWorktrees clicks the status bar's worktree button of the window's
// folder and waits until the worktrees are read.
func openWorktrees(t *testing.T, hs *harness) {
	t.Helper()
	if err := hs.tt.Click(locales.WorktreeStatusTip(filepath.Base(hs.a.root))); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	awaitWorktrees(t, hs)
}

// awaitWorktrees draws frames until the worktrees the box opened on are read.
func awaitWorktrees(t *testing.T, hs *harness) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for hs.a.worktrees.loading && time.Now().Before(deadline) {
		hs.settle()
	}
	if hs.a.worktrees.loading {
		t.Fatal("the worktree box never finished reading the worktrees")
	}
}

func TestStatusBarWorktreeBox(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	openWorktrees(t, hs)
	if !a.worktrees.open || len(a.worktrees.trees) != 2 {
		t.Fatalf("box open %v with %d worktrees", a.worktrees.open, len(a.worktrees.trees))
	}
	// The one open comes first, and has its check; the other is by its folder.
	if first := a.worktrees.trees[0]; !first.Current || first.Branch != "main" {
		t.Fatalf("first worktree %+v is not the one open", first)
	}
	if !hs.tt.HasText("side") {
		t.Fatal("box does not name the linked worktree")
	}
	// Escape shuts the box and lets go of what it read.
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.worktrees.open || a.worktrees.trees != nil || a.worktrees.names != nil {
		t.Fatal("closed box kept its worktrees")
	}
}

func TestWorktreeBoxSwitchesToLinked(t *testing.T) {
	hs, run, side := worktreeFixture(t)
	openWorktrees(t, hs)
	if err := hs.tt.Click("side"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	want, err := filepath.EvalSymlinks(side)
	if err != nil {
		t.Fatal(err)
	}
	if hs.a.root != want {
		t.Fatalf("window shows %q, want %q", hs.a.root, want)
	}
	if strings.TrimSpace(run("symbolic-ref", "--short", "HEAD")) != "main" || hs.a.fails != 0 {
		t.Fatalf("switching changed the branch or failed %d times", hs.a.fails)
	}
	// The status bar names the linked folder it is in.
	if !hs.tt.HasText(filepath.Base(want)) {
		t.Fatal("status bar does not name the linked worktree")
	}
}

func TestWorktreeBoxCurrentOnlyCloses(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	root := a.root
	openWorktrees(t, hs)
	// Enter on the first row, the one open, shuts the box and does nothing else.
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.worktrees.open || a.root != root || a.fails != 0 {
		t.Fatalf("choosing the open worktree: open %v root %q failures %d", a.worktrees.open, a.root, a.fails)
	}
}

func TestWorktreeBoxMissingFolderIsReported(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	gone := filepath.Join(t.TempDir(), "gone")
	run("worktree", "add", "-b", "gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	root, fails := a.root, a.fails
	openWorktrees(t, hs)
	if err := hs.tt.Click("gone"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	// The failure is shown as a message, which the frames clear: it counts.
	if a.root != root || a.fails != fails+1 {
		t.Fatalf("missing folder: root %q, failures %d before %d", a.root, a.fails, fails)
	}
	// Nothing was pruned: Git still lists the worktree, and the branch
	// stays where it was.
	if list := run("worktree", "list"); !strings.Contains(list, "gone") {
		t.Fatalf("worktree list lost the missing worktree:\n%s", list)
	}
	if strings.TrimSpace(run("symbolic-ref", "--short", "HEAD")) != "main" {
		t.Fatal("missing folder changed the branch")
	}
}

func TestWorktreeBoxFuzzyAndCreate(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	openWorktrees(t, hs)
	// A query naming an existing worktree offers no creation.
	hs.tt.Type("side")
	hs.settle()
	for _, ch := range a.repository().worktreeChoices() {
		if ch.create != "" {
			t.Fatal("offered to create a worktree named like an existing one")
		}
	}
	if len(a.repository().worktreeChoices()) == 0 {
		t.Fatal("query side matched no worktree")
	}
	hs.tt.Key(ui.Cmd, ui.KeyA)
	hs.tt.Type("feature/new")
	hs.settle()
	if err := hs.tt.Click(locales.WorktreeCreateLabel("feature/new")); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	if a.worktrees.open {
		t.Fatal("box still open after creating")
	}
	if !slices.Contains(strings.Fields(run("branch", "--format=%(refname:short)")), "feature/new") {
		t.Fatal("branch feature/new was not made")
	}
	if !strings.HasSuffix(a.root, filepath.Join(".worktrees", "feature-new")) {
		t.Fatalf("window shows %q, not the new worktree folder", a.root)
	}
	if strings.TrimSpace(run("rev-parse", "HEAD")) != strings.TrimSpace(run("rev-parse", "feature/new")) {
		t.Fatal("new worktree does not start from the commit HEAD was at")
	}
}

func TestWorktreeBoxBusyDisabled(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	hs.a.git.busy = "held operation"
	hs.settle()
	if err := hs.tt.Click(locales.WorktreeStatusTip(filepath.Base(hs.root))); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.worktrees.open {
		t.Fatal("busy repository opened the worktree box")
	}
	hs.a.git.busy = ""
}

func TestWorktreeBoxStaysWithItsRepository(t *testing.T) {
	hs, sub, _ := submoduleHarness(t)
	a := hs.a
	parentRoot := a.repository().git.repo.Root
	if err := hs.tt.Click(locales.WorktreeStatusTip(filepath.Base(parentRoot))); err != nil {
		t.Fatal(err)
	}
	// Selecting the module before the worktrees arrive must not move the
	// box to it: its results belong to the repository it opened on.
	a.selectRepository(sub.git.repo.Root)
	hs.settle()
	awaitWorktrees(t, hs)
	if a.worktrees.repoRoot != parentRoot || len(a.worktrees.trees) != 1 {
		t.Fatalf("box of %q has %d worktrees", a.worktrees.repoRoot, len(a.worktrees.trees))
	}
}

// Opening and shutting the box, and switching between the worktrees, many
// times leaves no goroutine and no heap behind: what the box read is let go
// when it shuts.
func TestWorktreeBoxDoesNotLeak(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	cycle := func(n int) {
		for range n {
			openWorktrees(t, hs)
			hs.tt.Key(0, ui.KeyEscape)
			hs.settle()
		}
	}
	// Warm up what the first use of the box allocates once.
	cycle(2)
	base, baseHeap := settledGoroutines(hs), heapInUse()
	cycle(50)
	main := mustEval(t, hs.root)
	for range 5 {
		openWorktrees(t, hs)
		if err := hs.tt.Click("side"); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		openWorktrees(t, hs)
		if err := hs.tt.Click(filepath.Base(hs.root)); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	if a.root != main {
		t.Fatalf("switching back did not return to the main worktree: %q", a.root)
	}
	got := settledGoroutines(hs)
	grown := int64(heapInUse()) - int64(baseHeap)
	t.Logf("goroutines %d before, %d after 55 box cycles; heap in use %+d KB", base, got, grown>>10)
	if got > base {
		t.Fatalf("goroutines %d after the cycles, %d before", got, base)
	}
	if grown > 8<<20 {
		t.Fatalf("heap grew by %d KB over the cycles", grown>>10)
	}
	if a.worktrees.trees != nil || a.worktrees.names != nil {
		t.Fatal("closed box kept what it read")
	}
}

// settledGoroutines draws frames until background work ends, and reports
// how many goroutines run then.
func settledGoroutines(hs *harness) int {
	for range 20 {
		hs.settle()
	}
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}

// heapInUse collects garbage, and reports the heap in use after it.
func heapInUse() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// With HEAD detached, the row to create says which commit it starts from,
// by its short hash.
func TestWorktreeBoxCreateRowDetached(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	run("checkout", "-q", "--detach")
	hs.a.gitChanged()
	hs.settleGit()
	short := git.ShortHash(strings.TrimSpace(run("rev-parse", "HEAD")))
	openWorktrees(t, hs)
	hs.tt.Type("newone")
	hs.settle()
	if !hs.tt.HasText(locales.WorktreeFrom(short)) {
		t.Fatalf("create row does not say it is based on %s", short)
	}
}

// The worktree and the branch are in the title bar, not the status bar. At
// a narrow window, with the window controls at the left of the bar and the
// terminal panel there too, each of the buttons is on the window, and none
// of them lies over the command center, however long the branch name is.
func TestTitleBarWorktreeAndBranch(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	long := strings.Repeat("W", 64)
	// The menu button of a platform without the macOS menu bar sits in the
	// left group too; the window controls of the others take the bar's right.
	defer func(os string) { hostOS = os }(hostOS)
	for _, os := range []string{"darwin", "windows", "linux"} {
		hostOS = os
		right := float32(0)
		if os != "darwin" {
			right = 138
		}
		for _, zoom := range []float32{1, 1.5} {
			hs.tt.SetZoom(zoom)
			for _, branch := range []string{"main", long} {
				if branch != "main" {
					run("branch", "-m", branch)
					a.gitChanged()
					hs.settleGit()
				}
				name := strings.TrimSpace(run("symbolic-ref", "--short", "HEAD"))
				for _, layout := range []struct {
					right bool
					panel string
				}{{false, "bottom"}, {true, "left"}, {false, "left"}} {
					a.settings.SidebarRight, a.settings.PanelPosition = layout.right, layout.panel
					for _, left := range []float32{0, 68, 78} {
						for _, w := range []float32{640, 1200} {
							hs.tt.SetTitleBar(ui.TitleBar{Height: titleBarH, Left: left, Right: right})
							hs.tt.SetSize(int(w), 420)
							hs.settle()
							where := fmt.Sprintf("%s, zoom %v, branch %q, layout %+v, title bar left %v, width %v", os, zoom, name, layout, left, w)
							wt, ok := hs.tt.Find(locales.WorktreeStatusTip(filepath.Base(hs.root)))
							if !ok || wt.Y >= titleBarH {
								t.Fatalf("%s: worktree button missing from the title bar: %v %v", where, wt, ok)
							}
							br, ok := hs.tt.Find(locales.BranchLabel(name))
							if !ok || br.Y >= titleBarH {
								t.Fatalf("%s: branch button missing from the title bar: %v %v", where, br, ok)
							}
							cc, ok := hs.tt.Find(locales.QuickOpenLabel())
							if !ok {
								t.Fatalf("%s: command center missing", where)
							}
							for _, r := range []ui.Rect{wt, br} {
								if r.X < 0 || r.X+r.W > w {
									t.Fatalf("%s: button %v leaves the window", where, r)
								}
							}
							if overlaps(wt, cc) || overlaps(br, cc) || overlaps(wt, br) {
								t.Fatalf("%s: git buttons %v %v overlap the command center %v", where, wt, br, cc)
							}
						}
					}
				}
				run("branch", "-m", "main")
				a.gitChanged()
				hs.settleGit()
			}
			hs.tt.SetZoom(1)
		}
	}
	a.settings.SidebarRight, a.settings.PanelPosition = false, "bottom"
	hs.tt.SetTitleBar(ui.TitleBar{Height: titleBarH})
	hs.tt.SetSize(1200, 760)
	hs.settle()
}

// overlaps reports whether two rectangles share more than a sub-pixel edge.
func overlaps(a, b ui.Rect) bool {
	const eps = 0.5
	return a.X+eps < b.X+b.W && b.X+eps < a.X+a.W && a.Y+eps < b.Y+b.H && b.Y+eps < a.Y+a.H
}

// With no row to move among, the arrows and Enter do nothing, rather than
// choose a row that is not there.
func TestWorktreeBoxNoRowsArrowsAndEnter(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	openWorktrees(t, hs)
	hs.tt.Type("bad..no-match-xyz")
	hs.settle()
	for _, k := range []ui.Key{ui.KeyDown, ui.KeyUp, ui.KeyDown} {
		hs.tt.Key(0, k)
		hs.settle()
	}
	if !hs.a.worktrees.open || hs.a.fails != 0 {
		t.Fatalf("no rows: failures %d, open %v", hs.a.fails, hs.a.worktrees.open)
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if hs.a.fails != 0 {
		t.Fatalf("Enter with no rows: failures %d", hs.a.fails)
	}
}

// The row that creates a worktree names the base it starts from as the box
// opened, even when the branch changes while the box is open.
func TestWorktreeBoxCreateRowFrozenAtOpening(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	before := git.ShortHash(strings.TrimSpace(run("rev-parse", "HEAD")))
	openWorktrees(t, hs)
	run("checkout", "-q", "-b", "elsewhere")
	run("commit", "-q", "--allow-empty", "-m", "moved")
	hs.a.gitChanged()
	hs.settleGit()
	hs.tt.Type("newone")
	hs.settle()
	if !hs.tt.HasText(locales.WorktreeFrom("main (" + before + ")")) {
		t.Fatalf("create row does not keep the base of the opening: %v", hs.tt.Texts())
	}
}
