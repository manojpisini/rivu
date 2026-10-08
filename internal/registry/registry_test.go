package registry

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenAppliesPragmas(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	for _, tc := range []struct{ pragma, want string }{
		{"foreign_keys", "1"},
		{"busy_timeout", "5000"},
		{"journal_mode", "wal"},
	} {
		var got string
		if err := r.DB.QueryRow("PRAGMA " + tc.pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}

func TestCanonicalCleansPath(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "ws", "proj")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	want := canonical(sub)
	for _, in := range []string{
		sub + string(os.PathSeparator),
		filepath.Join(base, "ws", "other", "..", "proj"),
	} {
		if got := canonical(in); got != want {
			t.Errorf("canonical(%q) = %q, want %q", in, got, want)
		}
	}
	// Non-existent paths still Clean.
	if got := canonical(filepath.Join(base, "a", "..", "missing")); got != filepath.Join(base, "missing") {
		t.Errorf("canonical on missing path = %q", got)
	}
}

func TestUpsertFoldsEquivalentPaths(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	base := t.TempDir()
	real := filepath.Join(base, "app")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}

	first := Project{ID: "id-1", Name: "app", Slug: "app", Path: real, Channel: "sandbox", FlowStage: "current"}
	if err := r.Upsert(first); err != nil {
		t.Fatal(err)
	}
	// Trailing separator + parent-jitter + (on case-insensitive FS) case jitter
	// must resolve to the same row, not a duplicate.
	jitter := filepath.Join(base, "sub", "..", "app") + string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		jitter = strings.ToUpper(base[:1]) + jitter[1:]
	}
	second := Project{ID: "id-2", Name: "app2", Slug: "app2", Path: jitter, Channel: "sandbox", FlowStage: "current"}
	if err := r.Upsert(second); err != nil {
		t.Fatal(err)
	}

	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d rows, want 1", len(list))
	}
	if list[0].ID != "id-1" || list[0].Slug != "app" {
		t.Errorf("rescan replaced identity: %+v", list[0])
	}
}

func TestDiscoveryStoresRoot(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	scanned := Project{ID: "id-r", Name: "proj", Slug: "proj", Path: dir, Channel: "00_Source", FlowStage: "source", Root: filepath.FromSlash("/ws1"), LastScannedAt: time.Now(), OnDisk: true}
	if warns, err := r.ApplyDiscovery([]Project{scanned}); err != nil || len(warns) != 0 {
		t.Fatalf("insert: warns=%v err=%v", warns, err)
	}
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Root != filepath.FromSlash("/ws1") {
		t.Fatalf("root not stored on insert: %+v", got)
	}
	// Rescan under a different root refreshes it (discovery-owned field).
	scanned.Root = filepath.FromSlash("/ws2")
	scanned.LastScannedAt = time.Now().Add(time.Second)
	if warns, err := r.ApplyDiscovery([]Project{scanned}); err != nil || len(warns) != 0 {
		t.Fatalf("update: warns=%v err=%v", warns, err)
	}
	if got, err = r.List(); err != nil || len(got) != 1 || got[0].Root != filepath.FromSlash("/ws2") {
		t.Fatalf("root not refreshed on update: %+v err=%v", got, err)
	}
}

func TestDiscoverNeverTouchesRegistryOwnedFields(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dir := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	full := Project{ID: "id-1", Name: "My Proj", Slug: "my-proj", Path: dir, Channel: "sandbox", FlowStage: "building", Language: "go", CreatedAt: created, OnDisk: true, Registered: true}
	if err := r.Upsert(full); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHealth("id-1", 88); err != nil {
		t.Fatal(err)
	}

	// Scanner output: zero health, folder-derived name/stage, new language.
	scanned := Project{ID: "fresh-id", Name: "folder", Slug: "folder", Path: dir, Channel: "sandbox", FlowStage: "source", Language: "rust", Stack: []string{"cargo"}, HasGit: true, LastScannedAt: time.Now(), OnDisk: true}
	if warns, err := r.ApplyDiscovery([]Project{scanned}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}

	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	p := got[0]
	if p.ID != "id-1" || p.Slug != "my-proj" {
		t.Errorf("identity overwritten: id=%q slug=%q", p.ID, p.Slug)
	}
	if p.Name != "My Proj" || p.FlowStage != "building" {
		t.Errorf("registry-owned fields overwritten: name=%q stage=%q", p.Name, p.FlowStage)
	}
	if p.HealthScore != 88 {
		t.Errorf("health wiped to %d", p.HealthScore)
	}
	if !p.CreatedAt.Equal(created) {
		t.Errorf("created_at overwritten: %v", p.CreatedAt)
	}
	if p.Language != "rust" || !p.HasGit || p.Stack == nil || p.Stack[0] != "cargo" {
		t.Errorf("discovery fields not refreshed: %+v", p)
	}
}

func TestDuplicateFolderNamesGetStableDistinctSlugs(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	for _, ch := range []string{"sandbox", "archive"} {
		if err := os.MkdirAll(filepath.Join(root, ch, "app"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	scan := func() []Project {
		var ps []Project
		for i, ch := range []string{"sandbox", "archive"} {
			ps = append(ps, Project{ID: fmt.Sprintf("id-%d", i), Name: "app", Slug: "app", Path: filepath.Join(root, ch, "app"), Channel: ch, FlowStage: "source", CreatedAt: now, OnDisk: true})
		}
		return ps
	}
	if warns, err := r.ApplyDiscovery(scan()); err != nil || len(warns) != 0 {
		t.Fatalf("first scan: warns=%v err=%v", warns, err)
	}
	// Rescan must not shuffle the persisted slugs.
	if warns, err := r.ApplyDiscovery(scan()); err != nil || len(warns) != 0 {
		t.Fatalf("rescan: warns=%v err=%v", warns, err)
	}

	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d rows, want 2", len(list))
	}
	slugs := map[string]bool{}
	for _, p := range list {
		slugs[p.Slug] = true
	}
	if !slugs["app"] || !slugs["app-2"] {
		t.Errorf("slugs = %v, want app and app-2", slugs)
	}
}

func TestApplyDiscoveryWarnsInsteadOfAborting(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	// Same primary key on both rows: second insert must warn, not abort.
	ps := []Project{
		{ID: "dup", Name: "a", Slug: "a", Path: a, Channel: "sandbox", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "dup", Name: "b", Slug: "b", Path: b, Channel: "sandbox", FlowStage: "source", CreatedAt: now, OnDisk: true},
	}
	warns, err := r.ApplyDiscovery(ps)
	if err != nil {
		t.Fatalf("ApplyDiscovery fatal: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warns))
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "a" {
		t.Errorf("first project not committed: %+v", list)
	}
}

func TestScanFlagsVanishedProjectsMissing(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	gone := filepath.Join(root, "gone")
	stays := filepath.Join(root, "stays")
	now := time.Now()
	for _, d := range []string{gone, stays} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for i, d := range []string{gone, stays} {
		if err := r.Upsert(Project{ID: fmt.Sprintf("id-%d", i), Name: "p", Slug: fmt.Sprintf("p-%d", i), Path: d, Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true, Registered: true}); err != nil {
			t.Fatal(err)
		}
	}

	// Next scan sees only "stays": "gone" must be flagged missing.
	if warns, err := r.ApplyDiscovery([]Project{{ID: "id-1", Name: "p", Slug: "p-1", Path: stays, Channel: "00_Source", FlowStage: "source", LastScannedAt: now, OnDisk: true}}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Missing) != 1 || st.Missing[0].Slug != "p-0" {
		t.Errorf("missing = %+v, want p-0", st.Missing)
	}
	if p, _ := r.Find("p-1"); !p.OnDisk {
		t.Errorf("found project flagged missing: %+v", p)
	}
}

func TestStatesCategorizeMismatches(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	rows := []Project{
		{ID: "m", Name: "m", Slug: "m", Path: "/m", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: false, Registered: true},
		{ID: "u", Name: "u", Slug: "u", Path: "/u", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true, Registered: false},
		{ID: "s", Name: "s", Slug: "s", Path: "/s", Channel: "00_Source", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true},
		{ID: "ok", Name: "ok", Slug: "ok", Path: "/ok", Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true},
	}
	if _, err := r.ApplyDiscovery(rows); err != nil {
		t.Fatal(err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Missing) != 1 || st.Missing[0].ID != "m" {
		t.Errorf("missing = %+v", st.Missing)
	}
	if len(st.Unregistered) != 1 || st.Unregistered[0].ID != "u" {
		t.Errorf("unregistered = %+v", st.Unregistered)
	}
	if len(st.StageMismatch) != 1 || st.StageMismatch[0].ID != "s" {
		t.Errorf("stageMismatch = %+v", st.StageMismatch)
	}
}

func TestDiscoveryRefreshesChannelForStageMismatch(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dir := filepath.Join(t.TempDir(), "moved")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Registered as active; folder now sits in 00_Source.
	p := Project{ID: "id-1", Name: "moved", Slug: "moved", Path: dir, Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true}
	if err := r.Upsert(p); err != nil {
		t.Fatal(err)
	}
	p.Channel = "00_Source"
	if warns, err := r.ApplyDiscovery([]Project{p}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.StageMismatch) != 1 {
		t.Fatalf("stageMismatch = %+v, want 1", st.StageMismatch)
	}
	got, _ := r.Find("moved")
	if got.Channel != "00_Source" || got.FlowStage != "active" {
		t.Errorf("channel not refreshed or stage changed: ch=%q flow=%q", got.Channel, got.FlowStage)
	}
}

func TestResolveLadder(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	for i, p := range []Project{
		{ID: "1111-aaaa", Name: "Web Shop", Slug: "web-shop", Path: "/w1", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "2222-bbbb", Name: "web api", Slug: "web-api", Path: "/w2", Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true},
		{ID: "3333-cccc", Name: "mobile", Slug: "mobile", Path: "/w3", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	// Exact slug.
	if p, err := r.Resolve("web-shop"); err != nil || p.Slug != "web-shop" {
		t.Errorf("exact slug: p=%+v err=%v", p, err)
	}
	// Case-insensitive exact name.
	if p, err := r.Resolve("WEB API"); err != nil || p.Slug != "web-api" {
		t.Errorf("ci name: p=%+v err=%v", p, err)
	}
	// Unique id prefix.
	if p, err := r.Resolve("1111"); err != nil || p.Slug != "web-shop" {
		t.Errorf("id prefix: p=%+v err=%v", p, err)
	}
	// Fuzzy substring.
	if p, err := r.Resolve("shop"); err != nil || p.Slug != "web-shop" {
		t.Errorf("fuzzy: p=%+v err=%v", p, err)
	}
	// Not found, with did-you-mean hint.
	_, err = r.Resolve("web-shpp")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "web-shop") {
		t.Errorf("missing did-you-mean hint: %v", err)
	}
}

func TestResolveAmbiguousListsCandidates(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	for i, p := range []Project{
		{ID: "id-1", Name: "web", Slug: "web-a", Path: "/a", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "id-2", Name: "web", Slug: "web-b", Path: "/b", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	_, err = r.Resolve("web")
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}
	if !strings.Contains(err.Error(), "web-a") || !strings.Contains(err.Error(), "web-b") {
		t.Errorf("candidates not listed: %v", err)
	}
}

func TestListUsesLifecycleOrder(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	// Insert in reverse lifecycle order; alphabetical would differ too.
	for i, flow := range []string{"delta", "research", "maintenance", "active", "source"} {
		p := Project{ID: fmt.Sprintf("id-%d", i), Name: "p", Slug: flow, Path: "/" + flow, Channel: "00_Source", FlowStage: flow, CreatedAt: now, OnDisk: true}
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %s: %v", flow, err)
		}
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source", "active", "maintenance", "research", "delta"}
	for i, w := range want {
		if list[i].FlowStage != w {
			t.Fatalf("order[%d] = %s, want %s (full: %v)", i, list[i].FlowStage, w, list)
		}
	}
}

func TestOpenRecordsTimestampAndActivity(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now().Add(-time.Hour)
	if err := r.Upsert(Project{ID: "id-1", Name: "p", Slug: "p", Path: "/p", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Second)
	if err := r.MarkOpened("id-1"); err != nil {
		t.Fatal(err)
	}
	if err := r.LogActivity("id-1", "opened"); err != nil {
		t.Fatal(err)
	}

	p, err := r.Find("p")
	if err != nil {
		t.Fatal(err)
	}
	if p.LastOpenedAt.Before(before) {
		t.Errorf("last_opened_at not updated: %v", p.LastOpenedAt)
	}
	var event, projectID string
	var occurred time.Time
	if err := r.DB.QueryRow(`SELECT project_id,event,occurred_at FROM activity_log`).Scan(&projectID, &event, &occurred); err != nil {
		t.Fatal(err)
	}
	if projectID != "id-1" || event != "opened" || occurred.Before(before) {
		t.Errorf("activity row = (%s, %s, %v)", projectID, event, occurred)
	}
}

func TestCurrentAndResolveEmptyQuery(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := r.Current(); err == nil {
		t.Error("Current with no setting should fail")
	}
	now := time.Now()
	if err := r.Upsert(Project{ID: "id-1", Name: "p", Slug: "p", Path: "/p", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetCurrent("id-1"); err != nil {
		t.Fatal(err)
	}
	p, err := r.Resolve("")
	if err != nil || p.ID != "id-1" {
		t.Errorf("Resolve(\"\") = %+v, %v; want id-1", p, err)
	}
	// Stale current id maps to ErrNotFound.
	if err := r.SetCurrent("ghost"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(""); !errors.Is(err, ErrNotFound) {
		t.Errorf("stale current: want ErrNotFound, got %v", err)
	}
}

func TestUpdatePathFlow(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	if err := r.Upsert(Project{ID: "id-1", Name: "p", Slug: "p", Path: "/old", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdatePathFlow("id-1", "/new", "01_Active", "active"); err != nil {
		t.Fatal(err)
	}
	p, err := r.Find("p")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != canonical("/new") || p.Channel != "01_Active" || p.FlowStage != "active" {
		t.Errorf("path/channel/flow not updated: %+v", p)
	}
	if err := r.UpdatePathFlow("ghost", "/x", "00_Source", "source"); err == nil {
		t.Error("update of missing project should fail")
	}
}

func TestChannelFlowMapping(t *testing.T) {
	for flow, ch := range map[string]string{
		"source": "00_Source", "active": "01_Active", "maintenance": "02_Maintenance",
		"research": "03_Research", "delta": "90_Delta",
	} {
		if got := ChannelForFlow(flow); got != ch {
			t.Errorf("ChannelForFlow(%q) = %q, want %q", flow, got, ch)
		}
		if got := FlowForChannel(ch); got != flow {
			t.Errorf("FlowForChannel(%q) = %q, want %q", ch, got, flow)
		}
	}
	if ChannelForFlow("bogus") != "00_Source" || FlowForChannel("junk") != "source" {
		t.Error("unknown inputs must fall back to source defaults")
	}
}

func TestTimeRoundTrip(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Zero time.Time must survive the round trip (R-11).
	if err := r.Upsert(Project{ID: "zero", Name: "z", Slug: "z", Path: "/z", Channel: "00_Source", FlowStage: "source"}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Find("z")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.IsZero() {
		t.Errorf("zero CreatedAt came back as %v", got.CreatedAt)
	}

	// A real timestamp survives to second precision.
	stamp := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := r.Upsert(Project{ID: "real", Name: "r", Slug: "r", Path: "/r", Channel: "00_Source", FlowStage: "source", CreatedAt: stamp, LastOpenedAt: stamp, LastScannedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	got, err = r.Find("r")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(stamp) || !got.LastOpenedAt.Equal(stamp) || !got.LastScannedAt.Equal(stamp) {
		t.Errorf("time drift: created=%v opened=%v scanned=%v, want %v", got.CreatedAt, got.LastOpenedAt, got.LastScannedAt, stamp)
	}
}

func TestConcurrentUpserts(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	const workers, each = 4, 5
	var wg sync.WaitGroup
	errs := make(chan error, workers*each)
	now := time.Now()
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				p := Project{ID: fmt.Sprintf("id-%d-%d", w, i), Name: "p", Slug: fmt.Sprintf("p-%d-%d", w, i), Path: fmt.Sprintf("/p/%d/%d", w, i), Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true}
				errs <- r.Upsert(p)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent upsert: %v", err)
		}
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != workers*each {
		t.Errorf("got %d rows, want %d", len(list), workers*each)
	}
}

func TestMigrateSetsUserVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := r.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Errorf("user_version = %d, want %d", v, len(migrations))
	}
	r.Close()

	// Re-open must be a no-op.
	if r, err = Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	for _, idx := range []string{"idx_projects_flow_stage", "idx_projects_last_opened", "idx_snapshots_project_taken"} {
		var n int
		if err := r.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("index %s missing", idx)
		}
	}
	r.Close()
}

func TestMigrateAdoptsLegacyDatabaseWithBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-versioning layout: tables present, user_version still 0.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,path TEXT NOT NULL UNIQUE,channel TEXT NOT NULL,flow_stage TEXT NOT NULL,language TEXT,stack TEXT,has_git INTEGER DEFAULT 0,has_bank INTEGER DEFAULT 0,has_map INTEGER DEFAULT 0,health_score INTEGER DEFAULT 0,created_at DATETIME,last_opened_at DATETIME,last_scanned_at DATETIME,on_disk INTEGER DEFAULT 1,registered INTEGER DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open legacy db: %v", err)
	}
	r.Close()
	if _, err := os.Stat(path + ".v0.bak"); err != nil {
		t.Errorf("expected backup before migrating legacy db: %v", err)
	}
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("Open accepted database from newer schema, want error")
	}
}

func TestForeignKeysCascade(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := r.DB.Exec(`INSERT INTO confluences(id,name) VALUES('c1','one')`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES('h1','missing-project',50,?)`, time.Now()); err == nil {
		t.Fatal("insert with unknown project_id succeeded, want FK violation")
	}
}

func TestUpsertAllowsDuplicateProjectNamesAtDifferentPaths(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	one := Project{ID: "one", Name: "admin", Slug: "admin", Path: filepath.Join("root", "web", "admin"), Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}
	two := Project{ID: "two", Name: "admin", Slug: "admin", Path: filepath.Join("root", "rust", "admin"), Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}

	if err := r.Upsert(one); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(two); err != nil {
		t.Fatalf("second duplicate-name project should be accepted: %v", err)
	}

	projects, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	if projects[0].Slug == projects[1].Slug {
		t.Fatalf("slugs must be unique, both were %q", projects[0].Slug)
	}
}

func TestUpsertKeepsStableSlugWhenRescanningSamePath(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	path := filepath.Join("root", "web", "admin")
	first := Project{ID: "one", Name: "admin", Slug: "admin", Path: path, Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}
	rescan := first
	rescan.ID = "new-random-id"

	if err := r.Upsert(first); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(rescan); err != nil {
		t.Fatal(err)
	}

	projects, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].ID != "one" {
		t.Fatalf("rescan replaced stable id: got %q", projects[0].ID)
	}
	if projects[0].Slug != "admin" {
		t.Fatalf("rescan changed stable slug: got %q", projects[0].Slug)
	}
}

func TestApplyFlowRecordsMoveAndActivity(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "id-flow", Name: "p", Slug: "p", Path: "/old", Channel: "00_Source", FlowStage: "source", CreatedAt: time.Now(), OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyFlow("id-flow", "/new", "01_Active", "active", true, false); err != nil {
		t.Fatal(err)
	}
	p, err := r.Find("p")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != canonical("/new") || p.Channel != "01_Active" || p.FlowStage != "active" || !p.HasBank || p.HasMap {
		t.Errorf("apply flow state wrong: %+v", p)
	}
	var events int
	if err := r.DB.QueryRow(`SELECT count(*) FROM activity_log WHERE project_id=? AND event='flowed'`, "id-flow").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("flowed activity rows = %d, want 1", events)
	}
	if err := r.ApplyFlow("ghost", "/x", "01_Active", "active", false, false); err == nil {
		t.Error("apply flow for a missing project should fail")
	}
}

func TestUpdateFlagsAndMarkMissing(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "id-flags", Name: "p", Slug: "p", Path: "/f", Channel: "00_Source", FlowStage: "source", CreatedAt: time.Now(), OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateFlags("id-flags", true, false); err != nil {
		t.Fatal(err)
	}
	p, _ := r.Find("p")
	if !p.HasBank || p.HasMap {
		t.Errorf("flags after UpdateFlags(true,false): %+v", p)
	}
	if err := r.UpdateFlags("id-flags", false, true); err != nil {
		t.Fatal(err)
	}
	p, _ = r.Find("p")
	if p.HasBank || !p.HasMap {
		t.Errorf("flags after UpdateFlags(false,true): %+v", p)
	}
	if err := r.MarkMissing("id-flags"); err != nil {
		t.Fatal(err)
	}
	p, _ = r.Find("p")
	if p.OnDisk {
		t.Error("MarkMissing must set on_disk=0")
	}
	if err := r.MarkMissing("ghost"); err != nil {
		t.Errorf("MarkMissing on unknown id: %v", err)
	}
}

func TestSlugOrPathTaken(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "id-taken", Name: "p", Slug: "taken", Path: "/ws/taken", Channel: "00_Source", FlowStage: "source", CreatedAt: time.Now(), OnDisk: true}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		slug, path string
		want       bool
	}{
		{"taken", "/ws/other", true},
		{"other", "/ws/taken", true},
		{"other", filepath.Join("/ws", "..", "ws", "taken"), true},
		{"other", "/ws/free", false},
	} {
		got, err := r.SlugOrPathTaken(tc.slug, tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("SlugOrPathTaken(%q, %q) = %v, want %v", tc.slug, tc.path, got, tc.want)
		}
	}
}

// TestHealthSnapshotsOldestFirstWithLimit: the sparkline reader (P4.14)
// returns the newest rows oldest-first, honours the limit, and treats a
// missing project as empty.
func TestHealthSnapshotsOldestFirstWithLimit(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(t.TempDir(), "alpha"), FlowStage: "source"}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, score := range []int{40, 70, 55} {
		if _, err := r.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES(?,?,?,?)`,
			fmt.Sprintf("s%d", i), "p1", score, base.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.HealthSnapshots("p1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Score != 70 || got[1].Score != 55 {
		t.Fatalf("limited snapshots = %+v, want [70 55] oldest first", got)
	}
	all, err := r.HealthSnapshots("p1", 0) // default limit
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Score != 40 || all[2].Score != 55 {
		t.Fatalf("all snapshots = %+v, want 3 rows in time order", all)
	}
	none, err := r.HealthSnapshots("missing", 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("missing project: %+v err=%v, want empty", none, err)
	}
}

// TestAddSnapshotDedupesSameDayAndScore: repeat writes on the same
// calendar day with the same score are no-ops; a score change, a new
// day or a different project each adds a row; unknown projects fail
// the FK check (P5.05).
func TestAddSnapshotDedupesSameDayAndScore(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, p := range []Project{
		{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(t.TempDir(), "alpha"), FlowStage: "source"},
		{ID: "p2", Name: "Beta", Slug: "beta", Path: filepath.Join(t.TempDir(), "beta"), FlowStage: "source"},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}
	day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	steps := []struct {
		name  string
		pid   string
		score int
		at    time.Time
		want  int
	}{
		{"first write", "p1", 80, day1.Add(9 * time.Hour), 1},
		{"same day same score", "p1", 80, day1.Add(12 * time.Hour), 1},
		{"same day new score", "p1", 75, day1.Add(15 * time.Hour), 2},
		{"same score next day", "p1", 80, day1.AddDate(0, 0, 1), 3},
		{"other project same day score", "p2", 80, day1.Add(9 * time.Hour), 1},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			if err := r.AddSnapshot(s.pid, s.score, s.at); err != nil {
				t.Fatalf("AddSnapshot: %v", err)
			}
			got, err := r.HealthSnapshots(s.pid, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != s.want {
				t.Fatalf("rows for %s = %d, want %d", s.pid, len(got), s.want)
			}
		})
	}
	if err := r.AddSnapshot("missing", 80, day1); err == nil {
		t.Fatal("unknown project accepted, want FK error")
	}
}

// TestPruneSnapshotsDropsOlderThanCutoff: rows before the cutoff go,
// rows at or after it stay, and pruning an empty table is a no-op
// (P5.06).
func TestPruneSnapshotsDropsOlderThanCutoff(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(t.TempDir(), "alpha"), FlowStage: "source"}); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id string
		at time.Time
	}{
		{"old", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		{"edge", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{"recent", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)},
	}
	for i, row := range rows {
		if _, err := r.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES(?,?,?,?)`,
			row.id, "p1", 50+i, row.at); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.PruneSnapshots(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	got, err := r.HealthSnapshots("p1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Score != 51 || got[1].Score != 52 {
		t.Fatalf("after prune = %+v, want scores [51 52] (cutoff row kept)", got)
	}
	if err := r.PruneSnapshots(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("prune all: %v", err)
	}
	if left, _ := r.HealthSnapshots("p1", 0); len(left) != 0 {
		t.Fatalf("after prune-all = %+v, want empty", left)
	}
}

// TestActivityDailyBucketsByCivilDay: one slot per day, zero-filled,
// oldest first; events outside the window are dropped (P4.19).
func TestActivityDailyBucketsByCivilDay(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Upsert(Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(t.TempDir(), "alpha"), FlowStage: "source"}); err != nil {
		t.Fatal(err)
	}
	const days = 30
	start := civilDay(time.Now()).AddDate(0, 0, -(days - 1))
	// offsets: first day of the window, next day, today, and one too old
	for i, off := range []int{0, 1, 29, 40} {
		at := start.AddDate(0, 0, off).Add(12 * time.Hour)
		if _, err := r.DB.Exec(`INSERT INTO activity_log(id,project_id,event,occurred_at) VALUES(?,?,?,?)`,
			fmt.Sprintf("e%d", i), "p1", "opened", at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.ActivityDaily(days)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != days {
		t.Fatalf("len = %d, want %d", len(got), days)
	}
	if got[0] != 1 || got[1] != 1 || got[29] != 1 {
		t.Fatalf("buckets = %v, want events at 0, 1 and 29", got)
	}
	if got[5] != 0 {
		t.Errorf("quiet day = %d, want zero-filled", got[5])
	}
	sum := 0
	for _, c := range got {
		sum += c
	}
	if sum != 3 {
		t.Errorf("sum = %d, want 3 (the offset-40 event is out of window)", sum)
	}
	empty, err := r.ActivityDaily(0)
	if err != nil || len(empty) != 0 {
		t.Errorf("ActivityDaily(0) = %v, %v; want empty", empty, err)
	}
}
