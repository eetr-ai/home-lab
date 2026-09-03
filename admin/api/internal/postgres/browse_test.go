package postgres

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// encodeRaw base64s an arbitrary string the way a cursor is encoded, so a test can
// hand decodeCursor a well-formed token whose contents are not a cursor.
func encodeRaw(raw string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// The cursor is the one thing on this path that crosses to the browser and back,
// so a value that contains the delimiter, or a token this server did not mint,
// are the cases that matter.
func TestCursorRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"single", []string{"42"}},
		{"composite", []string{"2024-01-02 03:04:05+00", "7"}},
		{"value contains a comma", []string{"a,b", "c"}},
		{"value contains a quote", []string{`x"y`}},
		{"empty string is a value", []string{""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := decodeCursor(encodeCursor(test.values))
			if err != nil {
				t.Fatalf("decode a cursor this package encoded: %v", err)
			}
			if len(decoded) != len(test.values) {
				t.Fatalf("got %d values, want %d", len(decoded), len(test.values))
			}
			for i := range test.values {
				if decoded[i] != test.values[i] {
					t.Errorf("value %d: got %q, want %q", i, decoded[i], test.values[i])
				}
			}
		})
	}
}

func TestDecodeCursorRejectsMalformed(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{"not base64", "not valid base64!!"},
		{"base64 of non-JSON", encodeRaw("this is not json")},
		{"base64 of a JSON object", encodeRaw(`{"not":"an array"}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeCursor(test.cursor); !errors.Is(err, ErrInvalidName) {
				t.Errorf("got %v, want an ErrInvalidName", err)
			}
		})
	}
}

func TestBuildBrowseSQL(t *testing.T) {
	single := []pkColumn{{Name: "id", Type: "bigint"}}
	composite := []pkColumn{{Name: "tenant", Type: "uuid"}, {Name: "seq", Type: "integer"}}

	t.Run("first page orders by the key and fetches one past the page", func(t *testing.T) {
		exec, display, err := buildBrowseSQL("public", "users", single, false, 200)
		if err != nil {
			t.Fatal(err)
		}
		// No WHERE on the first page: there is nothing to continue from yet.
		if strings.Contains(exec, "WHERE") {
			t.Errorf("first page should have no WHERE clause: %s", exec)
		}
		mustContain(t, exec, `FROM "public"."users"`)
		mustContain(t, exec, `ORDER BY "id"`)
		// One past the page, so a full page signals a next one.
		mustContain(t, exec, "LIMIT 201")
		// The key rides along as text to become the next cursor.
		mustContain(t, exec, `"id"::text`)
		// The console shows the clean statement, capped at the page size itself.
		mustContain(t, display, `SELECT * FROM "public"."users" ORDER BY "id" LIMIT 200`)
		if strings.Contains(display, "::text") || strings.Contains(display, "__cursor") {
			t.Errorf("the shown statement should carry no cursor bookkeeping: %s", display)
		}
	})

	t.Run("a later page compares the whole key against the cursor", func(t *testing.T) {
		exec, _, err := buildBrowseSQL("public", "events", composite, true, 200)
		if err != nil {
			t.Fatal(err)
		}
		// Row-value comparison over the key in key order, matching the ORDER BY —
		// the keyset step, not an OFFSET.
		mustContain(t, exec, `WHERE ("tenant", "seq") > ($1::uuid, $2::integer)`)
		mustContain(t, exec, `ORDER BY "tenant", "seq"`)
	})

	t.Run("a keyless relation is a single capped page", func(t *testing.T) {
		exec, display, err := buildBrowseSQL("public", "audit_log", nil, false, 200)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(exec, "ORDER BY") || strings.Contains(exec, "__cursor") {
			t.Errorf("a keyless relation has no key to order or page by: %s", exec)
		}
		mustContain(t, exec, "LIMIT 201")
		mustContain(t, display, `SELECT * FROM "public"."audit_log" LIMIT 200`)
	})

	t.Run("an embedded quote is escaped, not refused, so a real name still browses", func(t *testing.T) {
		exec, _, err := buildBrowseSQL(`we"ird`, "My Orders", single, false, 200)
		if err != nil {
			t.Fatal(err)
		}
		// The quote is doubled and the whole name wrapped — quote_ident semantics —
		// so the identifier cannot break out of its quotes into the statement.
		mustContain(t, exec, `FROM "we""ird"."My Orders"`)
	})

	t.Run("a null byte in an identifier is refused", func(t *testing.T) {
		if _, _, err := buildBrowseSQL("public", "a\x00b", single, false, 200); !errors.Is(err, ErrInvalidName) {
			t.Errorf("a table with a null byte should be refused: %v", err)
		}
		badKey := []pkColumn{{Name: "", Type: "bigint"}}
		if _, _, err := buildBrowseSQL("public", "users", badKey, false, 200); !errors.Is(err, ErrInvalidName) {
			t.Errorf("an empty key column name should be refused: %v", err)
		}
	})
}

// A uuid decodes to a raw [16]byte, and rendering it as a list of numbers is the
// difference between a readable primary-key column and an unreadable one.
func TestRenderValueFormatsUUID(t *testing.T) {
	id := [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	if got, want := renderValue(id), "6ba7b810-9dad-11d1-80b4-00c04fd430c8"; got != want {
		t.Errorf("renderValue(uuid) = %q, want %q", got, want)
	}
	if got := renderValue(nil); got != "NULL" {
		t.Errorf("renderValue(nil) = %q, want NULL", got)
	}
	if got := renderValue(int64(42)); got != "42" {
		t.Errorf("renderValue(42) = %q, want 42", got)
	}
}

func TestResolvePageSize(t *testing.T) {
	tests := []struct {
		requested int
		want      int
	}{
		{requested: 0, want: defaultBrowsePageSize},
		{requested: -5, want: defaultBrowsePageSize},
		{requested: 25, want: 25},
		{requested: browsePageSize, want: browsePageSize},
		{requested: browsePageSize + 1, want: browsePageSize},
		{requested: 10_000, want: browsePageSize},
	}
	for _, test := range tests {
		if got := resolvePageSize(test.requested); got != test.want {
			t.Errorf("resolvePageSize(%d) = %d, want %d", test.requested, got, test.want)
		}
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to find %q in:\n%s", needle, haystack)
	}
}
