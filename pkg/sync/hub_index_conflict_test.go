package sync

import (
	"strings"
	"testing"

	"github.com/okf-memory/okf-agent-memory/pkg/okf"
)

// Two devices (or agents) that create *different* concepts touch no common
// concept file, but `okf create` also rewrites the parent folder's index.md.
// The reconcile engine auto-merges only log.md, so a shared index.md becomes a
// same-file collision even though the work is disjoint.

// createConcept mirrors `okf create`: write the concept and run automatic
// index and log bookkeeping.
func createConcept(t *testing.T, dir, id, title string) {
	t.Helper()
	c := &okf.Concept{
		ID:          id,
		Path:        id + ".md",
		Type:        "Decision",
		Title:       title,
		Description: title + " summary.",
		Body:        "# " + title + "\n",
	}
	if err := okf.SaveConcept(dir, c, okf.SaveOptions{IsNew: true, AutoLog: true, AutoIndex: true}); err != nil {
		t.Fatalf("SaveConcept(%s): %v", id, err)
	}
}

// twoDevicesFromSeed returns two synced bundles that both contain the seed concepts.
func twoDevicesFromSeed(t *testing.T, seeds ...string) (*hubFixture, string, string) {
	t.Helper()
	f := newHubFixture(t)
	dirA := t.TempDir()
	if err := okf.InitBundle(dirA); err != nil {
		t.Fatalf("InitBundle: %v", err)
	}
	for _, id := range seeds {
		createConcept(t, dirA, id, "Seed "+id)
	}
	f.push(dirA, "seed")

	dirB := t.TempDir()
	f.pull(dirB)
	return f, dirA, dirB
}

func assertConverged(t *testing.T, dirs map[string]string, indexPath string, wantInIndex []string) {
	t.Helper()
	for name, dir := range dirs {
		if c := conflictFiles(t, dir); len(c) != 0 {
			t.Errorf("%s: disjoint creates should not conflict, found %v", name, c)
		}
		index := readFile(t, dir, indexPath)
		for _, want := range wantInIndex {
			if !strings.Contains(index, want) {
				t.Errorf("%s: %s is missing %q:\n%s", name, indexPath, want, index)
			}
		}
	}
}

// Fails today: both creates rewrite decisions/index.md.
func TestHub_DisjointCreatesInSameFolderDoNotConflict(t *testing.T) {
	f, dirA, dirB := twoDevicesFromSeed(t, "decisions/seed")

	createConcept(t, dirA, "decisions/alpha", "Alpha")
	createConcept(t, dirB, "decisions/beta", "Beta")
	f.sync(dirA, "A creates alpha")
	f.sync(dirB, "B creates beta")
	f.sync(dirA, "A catches up")

	assertConverged(t, map[string]string{"A": dirA, "B": dirB}, "decisions/index.md",
		[]string{"alpha.md", "beta.md", "seed.md"})
}

// Control: the same disjoint work in different folders shares no index file
// and merges cleanly today, isolating the shared index.md as the cause above.
func TestHub_DisjointCreatesInDifferentFoldersDoNotConflict(t *testing.T) {
	f, dirA, dirB := twoDevicesFromSeed(t, "decisions/seed", "facts/seed")

	createConcept(t, dirA, "decisions/alpha", "Alpha")
	createConcept(t, dirB, "facts/beta", "Beta")
	f.sync(dirA, "A creates alpha")
	f.sync(dirB, "B creates beta")
	f.sync(dirA, "A catches up")

	dirs := map[string]string{"A": dirA, "B": dirB}
	assertConverged(t, dirs, "decisions/index.md", []string{"alpha.md"})
	assertConverged(t, dirs, "facts/index.md", []string{"beta.md"})
}
