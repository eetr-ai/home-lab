package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// minPasswordLength is the shortest password this panel will set. The same bar
// as the database slices, for the same reason: past the NetworkPolicy, the
// password is the whole boundary.
const minPasswordLength = 16

// defaultUser is the server's built-in account, and the one this panel connects
// as. It is never created, changed or removed from here: its password is the
// platform's Secret, its line in the aclfile is rewritten from that Secret on
// every start, and losing it would lock the panel — and the web's refresh lock —
// out of the server.
const defaultUser = "default"

// ruleReset is the ACL rule that returns a user to nothing: no password, no keys,
// no channels, no commands, disabled.
const ruleReset = "reset"

// The permission presets a user can have, and what each means to Redis.
//
// Every user is confined to its own key prefix and the matching pub/sub
// channels; a preset says what it may do there. Rules are built here from these
// lists and a validated prefix, and never accepted from the caller, so the panel
// cannot be used to hand out `allkeys +@all` by typing it into a form.
//
// readwrite is everything except @dangerous — the category that holds FLUSHALL,
// KEYS, CONFIG, DEBUG, and the rest of what reaches past a prefix or takes the
// server down. INFO is in that category too, and is given back by name, because
// client libraries read the server version from it on connect.
//
// readonly is @read plus the connection housekeeping (AUTH, HELLO, PING, SELECT,
// CLIENT SETNAME), again minus @dangerous, which is what removes CLIENT KILL from
// @connection. ORDER MATTERS in ACL rules — a later rule overrides an earlier one
// — so the subtraction comes last.
var presets = map[string][]string{
	presetReadWrite: {"+@all", "-@dangerous", "+info"},
	presetReadOnly:  {"+@read", "+@connection", "-@dangerous"},
}

// The presets' names. presetCustom is never accepted, only reported: it is what a
// user created by hand with rules the panel did not write looks like.
const (
	presetReadWrite = "readwrite"
	presetReadOnly  = "readonly"
	presetCustom    = "custom"
)

// userNamePattern is what a user name may look like. Redis accepts nearly
// anything that is not a space; this accepts what is also safe as a label, a
// file line and a Secret key, which is everywhere the name ends up.
var userNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// prefixPattern is what a key prefix may look like. Deliberately free of every
// glob character (* ? [ ] \), because the prefix becomes the `~<prefix>*` pattern
// Redis matches keys against, and a `*` in the middle of one would quietly widen
// it to keys that belong to somebody else.
var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,63}$`)

// userRepository is the ACL half of what this service needs from Redis.
type userRepository interface {
	// ACLFile is the path of the aclfile the server loaded, or empty when it was
	// started without one.
	ACLFile(ctx context.Context) (string, error)
	// ACLList answers ACL LIST: one line per user, in the server's own syntax.
	ACLList(ctx context.Context) ([]string, error)
	ACLSetUser(ctx context.Context, name string, rules []string) error
	// ACLDelUser reports how many users it removed.
	ACLDelUser(ctx context.Context, name string) (int64, error)
	ACLSave(ctx context.Context) error
}

// ListUsers returns every user on the server, the default one included, and
// whether the server keeps them across a restart.
func (s *Service) ListUsers(ctx context.Context) (UserList, error) {
	file, err := s.repo.ACLFile(ctx)
	if err != nil {
		return UserList{}, err
	}
	lines, err := s.repo.ACLList(ctx)
	if err != nil {
		return UserList{}, err
	}

	ret := UserList{Persistent: file != "", Users: []User{}}
	for _, line := range lines {
		if user, ok := parseACLLine(line); ok {
			ret.Users = append(ret.Users, user)
		}
	}
	return ret, nil
}

// CreateUser creates an app's user, confined to its prefix.
func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest) (User, error) {
	if err := validateUserName(req.Name); err != nil {
		return User{}, err
	}
	if len(req.Password) < minPasswordLength {
		return User{}, fmt.Errorf("%w: a password must be at least %d characters", ErrWeakPassword, minPasswordLength)
	}
	rules, err := buildRules(req.Prefix, req.Preset, []string{passwordRule(req.Password)})
	if err != nil {
		return User{}, err
	}
	if err := s.requirePersistence(ctx); err != nil {
		return User{}, err
	}

	if _, found, err := s.findUser(ctx, req.Name); err != nil {
		return User{}, err
	} else if found {
		return User{}, fmt.Errorf("%w: there is already a user %q", ErrAlreadyExists, req.Name)
	}

	// Undoing a creation is deleting it, which needs no memory of what was there.
	undo := func(ctx context.Context) error {
		_, err := s.repo.ACLDelUser(ctx, req.Name)
		return err
	}
	if err := s.apply(ctx, func() error { return s.repo.ACLSetUser(ctx, req.Name, rules) }, undo); err != nil {
		return User{}, err
	}
	return User{Name: req.Name, Enabled: true, Prefix: req.Prefix, Preset: req.Preset, Editable: true}, nil
}

// UpdateUser replaces a user's prefix and preset and, optionally, its password.
//
// An empty password keeps the current one. Redis cannot hand a password back —
// only its hash — so the hash is what is carried over, which is exactly what an
// edit that does not touch the password should do.
func (s *Service) UpdateUser(ctx context.Context, name string, req UpdateUserRequest) (User, error) {
	if err := validateUserName(name); err != nil {
		return User{}, err
	}
	if req.Password != "" && len(req.Password) < minPasswordLength {
		return User{}, fmt.Errorf("%w: a password must be at least %d characters", ErrWeakPassword, minPasswordLength)
	}
	if err := s.requirePersistence(ctx); err != nil {
		return User{}, err
	}

	line, found, err := s.findUser(ctx, name)
	if err != nil {
		return User{}, err
	}
	if !found {
		return User{}, fmt.Errorf("%w: there is no user %q", ErrNotFound, name)
	}

	passwords := []string{passwordRule(req.Password)}
	if req.Password == "" {
		passwords = passwordHashes(line)
	}
	rules, err := buildRules(req.Prefix, req.Preset, passwords)
	if err != nil {
		return User{}, err
	}

	undo := func(ctx context.Context) error { return s.repo.ACLSetUser(ctx, name, restoreRules(line)) }
	if err := s.apply(ctx, func() error { return s.repo.ACLSetUser(ctx, name, rules) }, undo); err != nil {
		return User{}, err
	}
	return User{Name: name, Enabled: true, Prefix: req.Prefix, Preset: req.Preset, Editable: true}, nil
}

// DeleteUser removes a user. Connections already authenticated as it stay open
// until they close; Redis does not cut them.
func (s *Service) DeleteUser(ctx context.Context, name string) error {
	if err := validateUserName(name); err != nil {
		return err
	}
	if err := s.requirePersistence(ctx); err != nil {
		return err
	}

	line, found, err := s.findUser(ctx, name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: there is no user %q", ErrNotFound, name)
	}

	undo := func(ctx context.Context) error { return s.repo.ACLSetUser(ctx, name, restoreRules(line)) }
	remove := func() error {
		_, err := s.repo.ACLDelUser(ctx, name)
		return err
	}
	return s.apply(ctx, remove, undo)
}

// apply makes a change and writes it to the aclfile, undoing the change when the
// write fails.
//
// The two have to agree. A user that exists in memory and not on disk works
// until the next restart and then is gone — an app that authenticates fine for a
// week and then does not, with nothing in between to say why. So if ACL SAVE
// fails, the change is reverted in memory and the operator is told, and what the
// server is doing stays what it will do after a restart.
func (s *Service) apply(ctx context.Context, change func() error, undo func(context.Context) error) error {
	if err := change(); err != nil {
		return err
	}
	saveErr := s.repo.ACLSave(ctx)
	if saveErr == nil {
		return nil
	}
	if undoErr := undo(context.WithoutCancel(ctx)); undoErr != nil {
		return fmt.Errorf("%w: %w: the change could not be saved (%w) and could not be undone (%w)",
			ErrNotSaved, errNotUndone, saveErr, undoErr)
	}
	return fmt.Errorf("%w: the change could not be written to the aclfile, so it was undone: %w", ErrNotSaved, saveErr)
}

// requirePersistence refuses to change users on a server that would forget them.
//
// That is a Redis started before the platform chart grew its aclfile. Creating a
// user there would work, and the app using it would stop authenticating at the
// next restart for no visible reason; refusing now, with the fix in the message,
// is the better failure.
func (s *Service) requirePersistence(ctx context.Context) error {
	file, err := s.repo.ACLFile(ctx)
	if err != nil {
		return err
	}
	if file == "" {
		return fmt.Errorf("%w: this Redis was started without an aclfile, so a user created now "+
			"would be gone after its next restart; upgrade the platform chart first", ErrNotPersistent)
	}
	return nil
}

// findUser returns a user's ACL LIST line, if there is one.
func (s *Service) findUser(ctx context.Context, name string) (string, bool, error) {
	lines, err := s.repo.ACLList(ctx)
	if err != nil {
		return "", false, err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "user" && fields[1] == name {
			return line, true, nil
		}
	}
	return "", false, nil
}

// validateUserName refuses a name this panel will not send, and the default user.
func validateUserName(name string) error {
	if name == defaultUser {
		return fmt.Errorf("%w: the default user is the platform's own credential and is managed by its Secret",
			ErrProtected)
	}
	if !userNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q is not a user name this panel will create: lowercase letters, digits, "+
			"dots, dashes and underscores, starting with a letter or digit", ErrInvalidRequest, name)
	}
	return nil
}

// buildRules turns a prefix, a preset and the password rules into the full rule
// list for ACL SETUSER.
//
// It starts with `reset`, so the result is the whole user rather than a change
// to whatever it was: nothing a previous edit granted can survive an edit that
// did not mean to keep it.
func buildRules(prefix, preset string, passwords []string) ([]string, error) {
	if !prefixPattern.MatchString(prefix) {
		return nil, fmt.Errorf("%w: %q is not a key prefix this panel will grant: letters, digits and . _ : / -, "+
			"and no glob characters", ErrInvalidRequest, prefix)
	}
	commands, ok := presets[preset]
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a preset; use %s or %s", ErrInvalidRequest, preset,
			presetReadWrite, presetReadOnly)
	}

	return slices.Concat(
		[]string{ruleReset, "on"},
		passwords,
		[]string{"~" + prefix + "*", "&" + prefix + "*"},
		commands,
	), nil
}

// passwordRule is the rule that sets a password, carrying its SHA-256 rather
// than the password.
//
// `#<hex>` is Redis's own form for a password it already has hashed, so the
// plaintext never crosses the wire to the server — it does not appear in the
// command, the slow log, or a MONITOR session. It is the same idea as the
// PostgreSQL slice sending a SCRAM verifier, and Redis would store exactly this
// hash either way.
func passwordRule(password string) string {
	sum := sha256.Sum256([]byte(password))
	return "#" + hex.EncodeToString(sum[:])
}

// passwordHashes returns the `#<hex>` rules from a user's ACL LIST line, which is
// how a password is carried across an edit that does not change it.
func passwordHashes(line string) []string {
	ret := []string{}
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "#") {
			ret = append(ret, field)
		}
	}
	return ret
}

// restoreRules turns an ACL LIST line back into the rules that recreate it.
//
// ACL LIST is written in the same rule syntax SETUSER accepts, so replaying the
// line after a `reset` puts the user back exactly as it was — including rules
// this panel did not write, for a user somebody made by hand.
func restoreRules(line string) []string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return []string{ruleReset}
	}
	return append([]string{ruleReset}, fields[2:]...)
}

// parseACLLine reads one ACL LIST line into a User.
//
// A user's prefix and preset are recognized only when its rules are exactly the
// shape buildRules writes. Anything else — several key patterns, a selector, a
// command list we did not write — is reported as custom and not editable here,
// rather than shown as the nearest preset, which would be a confident claim about
// a permission the user does not have.
func parseACLLine(line string) (User, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "user" {
		return User{}, false
	}
	rules := sortRules(fields[2:])
	ret := User{Name: fields[1], Preset: presetCustom, Enabled: rules.enabled, HasPassword: rules.hasPassword}

	if ret.Name == defaultUser || strings.Contains(line, "(") {
		return ret, true
	}
	prefix, ok := confinedPrefix(rules.keys, rules.channels)
	if !ok {
		return ret, true
	}
	for name, preset := range presets {
		if sameRules(rules.commands, preset) {
			ret.Prefix, ret.Preset, ret.Editable = prefix, name, true
			break
		}
	}
	return ret, true
}

// aclRules is an ACL LIST line's rules, sorted by what they govern.
type aclRules struct {
	enabled, hasPassword     bool
	keys, channels, commands []string
}

// sortRules sorts a user's rules into keys, channels and commands.
func sortRules(fields []string) aclRules {
	var ret aclRules
	for _, field := range fields {
		switch {
		case field == "on":
			ret.enabled = true
		case strings.HasPrefix(field, "#"):
			ret.hasPassword = true
		case strings.HasPrefix(field, "~"), strings.HasPrefix(field, "%"), field == "allkeys":
			ret.keys = append(ret.keys, field)
		case strings.HasPrefix(field, "&"), field == "allchannels":
			ret.channels = append(ret.channels, field)
		case strings.HasPrefix(field, "+"), strings.HasPrefix(field, "-"):
			ret.commands = append(ret.commands, field)
		}
	}
	return ret
}

// confinedPrefix reports the prefix of a user whose keys and channels are exactly
// one `~<prefix>*` and one `&<prefix>*`.
func confinedPrefix(keys, channels []string) (string, bool) {
	if len(keys) != 1 || len(channels) != 1 {
		return "", false
	}
	prefix, ok := strings.CutPrefix(keys[0], "~")
	if !ok {
		return "", false
	}
	prefix, ok = strings.CutSuffix(prefix, "*")
	if !ok || channels[0] != "&"+prefix+"*" || !prefixPattern.MatchString(prefix) {
		return "", false
	}
	return prefix, true
}

// sameRules compares command rules as a set, minus the `-@all` that `reset`
// implies and ACL LIST may or may not spell out.
//
// Order does matter to Redis, but these lists are only ever written by
// buildRules, in one order; comparing as sets is about tolerating how a version
// prints them, not about accepting a different meaning.
func sameRules(got, want []string) bool {
	normalize := func(rules []string) []string {
		ret := slices.DeleteFunc(slices.Clone(rules), func(rule string) bool { return rule == "-@all" })
		slices.Sort(ret)
		return slices.Compact(ret)
	}
	return slices.Equal(normalize(got), normalize(want))
}
