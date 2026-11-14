package redis

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeACL is a Redis ACL table: a user name to the rules it was last set with,
// rendered back by ACLList the way ACL LIST renders it — `user <name> <rules>`.
//
// It applies rules the way Redis does in the one respect these tests lean on:
// every rule list the service sends starts with `reset`, so the stored rules are
// whatever followed it.
type fakeACL struct {
	aclFile string
	users   map[string][]string
	// order keeps ACL LIST deterministic.
	order []string
	// saveErr is what the next ACL SAVE answers, and saves counts the ones that
	// succeeded, so a test can tell "saved" from "never asked to save".
	saveErr error
	saves   int
	// held is whether the ACL lock is taken, and unguarded counts writes that
	// reached the table while it was not: the service promises there are none.
	held      bool
	locks     int
	unguarded int
}

func newFakeACL(lines ...string) fakeACL {
	ret := fakeACL{aclFile: "/acl/users.acl", users: map[string][]string{}}
	for _, line := range lines {
		fields := strings.Fields(line)
		ret.users[fields[1]] = fields[2:]
		ret.order = append(ret.order, fields[1])
	}
	return ret
}

func (f *fakeACL) ACLFile(context.Context) (string, error) { return f.aclFile, nil }

func (f *fakeACL) Lock(context.Context, string, time.Duration) (func(context.Context), error) {
	f.held = true
	f.locks++
	return func(context.Context) { f.held = false }, nil
}

func (f *fakeACL) guard() {
	if !f.held {
		f.unguarded++
	}
}

func (f *fakeACL) ACLList(context.Context) ([]string, error) {
	ret := []string{}
	for _, name := range f.order {
		if rules, ok := f.users[name]; ok {
			ret = append(ret, strings.Join(append([]string{"user", name}, rules...), " "))
		}
	}
	return ret, nil
}

func (f *fakeACL) ACLSetUser(_ context.Context, name string, rules []string) error {
	f.guard()
	if _, ok := f.users[name]; !ok {
		f.order = append(f.order, name)
	}
	f.users[name] = slices.DeleteFunc(slices.Clone(rules), func(rule string) bool { return rule == "reset" })
	return nil
}

func (f *fakeACL) ACLDelUser(_ context.Context, name string) (int64, error) {
	f.guard()
	if _, ok := f.users[name]; !ok {
		return 0, nil
	}
	delete(f.users, name)
	return 1, nil
}

func (f *fakeACL) ACLSave(context.Context) error {
	f.guard()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	return nil
}

// A password long enough to pass, and a stand-in hash for lines that only need
// to look like what ACL LIST prints. testHash is not the hash of testPassword;
// the tests that care about the real one compute it with passwordRule.
const (
	testPassword = "a-perfectly-long-password"
	testHash     = "#3e2d1c55bfd7f9b5e0d0e0ac6c5ea3fd37a43a2f0d0fdc9a6e52a2cde0c4b9d0"
)

func TestPasswordRuleIsTheSHA256(t *testing.T) {
	// Checked against `printf %s ... | sha256sum`, which is what the platform's
	// startup script uses for the default user — the two have to agree on the
	// form, or the default user's line and an app user's would be different kinds
	// of thing.
	got := passwordRule("hello")
	want := "#2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("passwordRule(hello) = %s, want %s", got, want)
	}
	if strings.Contains(passwordRule(testPassword), testPassword) {
		t.Error("the password rule must not carry the password")
	}
}

func TestBuildRules(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		preset  string
		want    []string
		wantErr error
	}{
		{
			name:   "readwrite confines keys and channels to the prefix",
			prefix: "orders:",
			preset: presetReadWrite,
			want:   []string{"reset", "on", "#h", "~orders:*", "&orders:*", "+@all", "-@dangerous", "+info", "-scan", "-randomkey"},
		},
		{
			name:   "readonly subtracts dangerous last, so CLIENT KILL does not ride in on @connection",
			prefix: "cache",
			preset: presetReadOnly,
			want:   []string{"reset", "on", "#h", "~cache*", "&cache*", "+@read", "+@connection", "-@dangerous", "-scan", "-randomkey"},
		},
		{
			// A glob character in the middle of a prefix would widen ~prefix* to keys
			// belonging to somebody else, so every one of them is refused.
			name: "a glob in the prefix is refused", prefix: "app*:", preset: presetReadWrite, wantErr: ErrInvalidRequest,
		},
		{name: "a bracket in the prefix is refused", prefix: "app[1]", preset: presetReadWrite, wantErr: ErrInvalidRequest},
		{name: "a space would be a second rule", prefix: "app allkeys", preset: presetReadWrite, wantErr: ErrInvalidRequest},
		{name: "an empty prefix would be every key", prefix: "", preset: presetReadWrite, wantErr: ErrInvalidRequest},
		{name: "custom is reported, never accepted", prefix: "app:", preset: presetCustom, wantErr: ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRules(tt.prefix, tt.preset, []string{"#h"}, true)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("buildRules() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildRules() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseACLLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want User
	}{
		{
			name: "a user this panel wrote reads back as its prefix and preset",
			line: "user orders on " + testHash + " ~orders:* resetchannels &orders:* -@all +@all -@dangerous +info -scan -randomkey",
			want: User{Name: "orders", Enabled: true, HasPassword: true, Prefix: "orders:", Preset: presetReadWrite, Editable: true},
		},
		{
			// Same tokens, different meaning: with -@dangerous first, @connection
			// grants CLIENT KILL back (checked with ACL DRYRUN on Redis 8.10). Reading
			// it as readonly would be a confident claim about a permission it has.
			name: "the same rules in another order are custom",
			line: "user cache on #abc ~cache:* &cache:* -@dangerous +@connection +@read -scan -randomkey",
			want: User{Name: "cache", Enabled: true, HasPassword: true, Preset: presetCustom},
		},
		{
			name: "the implied -@all is the one difference tolerated",
			line: "user cache on #abc ~cache:* &cache:* -@all +@read +@connection -@dangerous -scan -randomkey",
			want: User{Name: "cache", Enabled: true, HasPassword: true, Prefix: "cache:", Preset: presetReadOnly, Editable: true},
		},
		{
			name: "the default user is custom and never editable",
			line: "user default on #abc ~* &* +@all",
			want: User{Name: "default", Enabled: true, HasPassword: true, Preset: presetCustom},
		},
		{
			name: "a hand-made user with two key patterns is custom",
			line: "user mixed on #abc ~a:* ~b:* &a:* +@all -@dangerous +info -scan -randomkey",
			want: User{Name: "mixed", Enabled: true, HasPassword: true, Preset: presetCustom},
		},
		{
			name: "one extra command rule is enough to be custom",
			line: "user extra on #abc ~a:* &a:* +@all -@dangerous +info -scan -randomkey +flushall",
			want: User{Name: "extra", Enabled: true, HasPassword: true, Preset: presetCustom},
		},
		{
			name: "a selector is custom",
			line: "user sel on #abc ~a:* &a:* +@read (~b:* +@write)",
			want: User{Name: "sel", Enabled: true, HasPassword: true, Preset: presetCustom},
		},
		{
			// No password is nopass or no way in, and the form can express neither.
			name: "a user with no password is not editable",
			line: "user off-one off ~a:* &a:* +@read +@connection -@dangerous -scan -randomkey",
			want: User{Name: "off-one", Preset: presetCustom},
		},
		{
			name: "a disabled user with a password is editable, and reads as disabled",
			line: "user paused off #abc ~a:* &a:* +@read +@connection -@dangerous -scan -randomkey",
			want: User{Name: "paused", HasPassword: true, Prefix: "a:", Preset: presetReadOnly, Editable: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseACLLine(tt.line)
			if !ok {
				t.Fatalf("parseACLLine() did not recognize %q", tt.line)
			}
			if got != tt.want {
				t.Errorf("parseACLLine() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	valid := CreateUserRequest{Name: "orders", Password: testPassword, Prefix: "orders:", Preset: presetReadWrite}

	tests := []struct {
		name     string
		acl      fakeACL
		req      CreateUserRequest
		wantErr  error
		wantUser bool
	}{
		{name: "a valid request creates and saves the user", acl: newFakeACL(), req: valid, wantUser: true},
		{
			name: "the default user is refused", acl: newFakeACL(),
			req:     CreateUserRequest{Name: "default", Password: testPassword, Prefix: "x", Preset: presetReadWrite},
			wantErr: ErrProtected,
		},
		{
			name: "a short password is refused", acl: newFakeACL(),
			req:     CreateUserRequest{Name: "orders", Password: "short", Prefix: "orders:", Preset: presetReadWrite},
			wantErr: ErrWeakPassword,
		},
		{
			name: "a taken name is refused", acl: newFakeACL("user orders on #abc ~orders:* &orders:* +@read"),
			req: valid, wantErr: ErrAlreadyExists, wantUser: true,
		},
		{
			// The whole reason for the platform chart's aclfile: a user created on a
			// server without one would be gone at its next restart.
			name: "a server without an aclfile is refused", acl: fakeACL{users: map[string][]string{}},
			req: valid, wantErr: ErrNotPersistent,
		},
		{
			name: "a failed save is undone, so memory and disk still agree",
			acl:  func() fakeACL { f := newFakeACL(); f.saveErr = errors.New("disk full"); return f }(),
			req:  valid, wantErr: ErrNotSaved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{fakeACL: tt.acl}
			_, err := NewService(repo).CreateUser(context.Background(), tt.req)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateUser() error = %v, want %v", err, tt.wantErr)
			}
			if _, exists := repo.users[tt.req.Name]; exists != tt.wantUser {
				t.Errorf("user exists afterwards = %v, want %v", exists, tt.wantUser)
			}
			if tt.wantErr == nil {
				if repo.saves != 1 {
					t.Errorf("saved %d times, want once", repo.saves)
				}
				rules := strings.Join(repo.users[tt.req.Name], " ")
				if strings.Contains(rules, testPassword) {
					t.Errorf("the plaintext password reached Redis: %s", rules)
				}
				if !strings.Contains(rules, passwordRule(testPassword)) {
					t.Errorf("the password hash did not reach Redis: %s", rules)
				}
			}
		})
	}
}

func TestBuildRulesKeepsADisabledUserDisabled(t *testing.T) {
	got, err := buildRules("a:", presetReadOnly, []string{"#h"}, false)
	if err != nil {
		t.Fatalf("buildRules() unexpected error: %v", err)
	}
	if got[1] != "off" {
		t.Errorf("rules = %v, want the second rule to be off", got)
	}
}

func TestUpdateUser(t *testing.T) {
	existing := "user orders on #oldhash ~orders:* &orders:* +@read +@connection -@dangerous -scan -randomkey"

	t.Run("a disabled user stays disabled through an edit", func(t *testing.T) {
		paused := "user orders off #oldhash ~orders:* &orders:* +@read +@connection -@dangerous -scan -randomkey"
		repo := &fakeRepo{fakeACL: newFakeACL(paused)}
		user, err := NewService(repo).UpdateUser(context.Background(), "orders",
			UpdateUserRequest{Prefix: "orders:v2:", Preset: presetReadOnly})
		if err != nil {
			t.Fatalf("UpdateUser() unexpected error: %v", err)
		}
		if rules := repo.users["orders"]; rules[0] != "off" || user.Enabled {
			t.Errorf("rules = %v, enabled = %v; want it still off", rules, user.Enabled)
		}
	})

	//nolint:gosec // ACL LIST lines with stand-in hashes, not credentials.
	refused := map[string]string{
		// Saving the form would set a password or drop nopass: either way the
		// app's way of authenticating changes without anyone asking for it.
		"a nopass user": "user legacy on nopass ~orders:* &orders:* +@read +@connection -@dangerous -scan -randomkey",
		"a custom user": "user orders on #h ~orders:* &orders:* +@all",
	}
	for name, line := range refused {
		t.Run(name+" is refused rather than flattened into a preset", func(t *testing.T) {
			repo := &fakeRepo{fakeACL: newFakeACL(line)}
			target := strings.Fields(line)[1]
			_, err := NewService(repo).UpdateUser(context.Background(), target,
				UpdateUserRequest{Prefix: "orders:", Preset: presetReadOnly})
			if !errors.Is(err, ErrProtected) {
				t.Fatalf("UpdateUser() error = %v, want %v", err, ErrProtected)
			}
			if got := strings.Join(repo.users[target], " "); got != strings.Join(strings.Fields(line)[2:], " ") {
				t.Errorf("the user changed to %q", got)
			}
		})
	}

	t.Run("an empty password keeps the hash that was there", func(t *testing.T) {
		repo := &fakeRepo{fakeACL: newFakeACL(existing)}
		_, err := NewService(repo).UpdateUser(context.Background(), "orders",
			UpdateUserRequest{Prefix: "orders:v2:", Preset: presetReadWrite})
		if err != nil {
			t.Fatalf("UpdateUser() unexpected error: %v", err)
		}
		got := strings.Join(repo.users["orders"], " ")
		want := "on #oldhash ~orders:v2:* &orders:v2:* +@all -@dangerous +info -scan -randomkey"
		if got != want {
			t.Errorf("rules = %q, want %q", got, want)
		}
	})

	t.Run("a new password replaces the old hash rather than adding to it", func(t *testing.T) {
		repo := &fakeRepo{fakeACL: newFakeACL(existing)}
		_, err := NewService(repo).UpdateUser(context.Background(), "orders",
			UpdateUserRequest{Prefix: "orders:", Preset: presetReadOnly, Password: testPassword})
		if err != nil {
			t.Fatalf("UpdateUser() unexpected error: %v", err)
		}
		got := strings.Join(repo.users["orders"], " ")
		if strings.Contains(got, "#oldhash") || !strings.Contains(got, passwordRule(testPassword)) {
			t.Errorf("rules = %q, want only the new hash", got)
		}
	})

	t.Run("a failed save puts the user back exactly as it was", func(t *testing.T) {
		acl := newFakeACL(existing)
		acl.saveErr = errors.New("read-only file system")
		repo := &fakeRepo{fakeACL: acl}
		_, err := NewService(repo).UpdateUser(context.Background(), "orders",
			UpdateUserRequest{Prefix: "everything:", Preset: presetReadWrite})
		if !errors.Is(err, ErrNotSaved) {
			t.Fatalf("UpdateUser() error = %v, want %v", err, ErrNotSaved)
		}
		lines, _ := repo.ACLList(context.Background())
		if lines[0] != existing {
			t.Errorf("after the undo the user is %q, want %q", lines[0], existing)
		}
	})

	t.Run("a user that is not there is not found", func(t *testing.T) {
		repo := &fakeRepo{fakeACL: newFakeACL()}
		_, err := NewService(repo).UpdateUser(context.Background(), "orders",
			UpdateUserRequest{Prefix: "orders:", Preset: presetReadOnly})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("UpdateUser() error = %v, want %v", err, ErrNotFound)
		}
	})
}

func TestDeleteUser(t *testing.T) {
	existing := "user orders on #h ~orders:* &orders:* +@read +@connection -@dangerous -scan -randomkey"

	t.Run("a user is removed and the removal saved", func(t *testing.T) {
		repo := &fakeRepo{fakeACL: newFakeACL(existing)}
		if err := NewService(repo).DeleteUser(context.Background(), "orders"); err != nil {
			t.Fatalf("DeleteUser() unexpected error: %v", err)
		}
		if _, ok := repo.users["orders"]; ok || repo.saves != 1 {
			t.Errorf("user still there = %v, saves = %d; want gone and saved once", ok, repo.saves)
		}
	})

	t.Run("a failed save brings the user back", func(t *testing.T) {
		acl := newFakeACL(existing)
		acl.saveErr = errors.New("gone")
		repo := &fakeRepo{fakeACL: acl}
		if err := NewService(repo).DeleteUser(context.Background(), "orders"); !errors.Is(err, ErrNotSaved) {
			t.Fatalf("DeleteUser() error = %v, want %v", err, ErrNotSaved)
		}
		if _, ok := repo.users["orders"]; !ok {
			t.Error("the user was not restored after the save failed")
		}
	})

	t.Run("the default user is refused", func(t *testing.T) {
		repo := &fakeRepo{fakeACL: newFakeACL("user default on #h ~* &* +@all")}
		if err := NewService(repo).DeleteUser(context.Background(), "default"); !errors.Is(err, ErrProtected) {
			t.Errorf("DeleteUser() error = %v, want %v", err, ErrProtected)
		}
		if _, ok := repo.users["default"]; !ok {
			t.Error("the default user was removed")
		}
	})
}

// Every write to the ACL table — the change, the save, and an undo after a
// failed save — has to happen while the lock is held, or two replicas can
// interleave them. The fake counts any that do not.
func TestEveryACLWriteHoldsTheLock(t *testing.T) {
	existing := "user orders on #h ~orders:* &orders:* +@read +@connection -@dangerous -scan -randomkey"
	failing := func() fakeACL { f := newFakeACL(existing); f.saveErr = errors.New("disk full"); return f }

	writes := map[string]struct {
		acl fakeACL
		run func(*Service) error
	}{
		"create": {newFakeACL(), func(s *Service) error {
			_, err := s.CreateUser(context.Background(),
				CreateUserRequest{Name: "cache", Password: testPassword, Prefix: "cache:", Preset: presetReadOnly})
			return err
		}},
		"update": {newFakeACL(existing), func(s *Service) error {
			_, err := s.UpdateUser(context.Background(), "orders", UpdateUserRequest{Prefix: "orders:", Preset: presetReadWrite})
			return err
		}},
		"delete": {newFakeACL(existing), func(s *Service) error { return s.DeleteUser(context.Background(), "orders") }},
		"a delete whose save fails and is undone": {failing(), func(s *Service) error {
			return s.DeleteUser(context.Background(), "orders")
		}},
	}

	for name, tt := range writes {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{fakeACL: tt.acl}
			_ = tt.run(NewService(repo))
			if repo.locks != 1 {
				t.Errorf("took the lock %d times, want once", repo.locks)
			}
			if repo.unguarded != 0 {
				t.Errorf("%d writes reached the ACL table without the lock", repo.unguarded)
			}
			if repo.held {
				t.Error("the lock was not released")
			}
		})
	}
}

func TestListUsersReportsPersistence(t *testing.T) {
	repo := &fakeRepo{fakeACL: fakeACL{users: map[string][]string{}}}
	list, err := NewService(repo).ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers() unexpected error: %v", err)
	}
	if list.Persistent {
		t.Error("a server without an aclfile must not be reported as persistent")
	}
	if list.Users == nil {
		t.Error("an empty list must be [] rather than null")
	}
}
