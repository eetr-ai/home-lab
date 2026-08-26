package mongo

import (
	"strings"
	"testing"
)

func TestValidateDatabaseName(t *testing.T) {
	accepted := []string{"app", "app_data", "_internal", "app2", "app-data", strings.Repeat("a", 63)}
	for _, name := range accepted {
		if err := validateDatabaseName(name); err != nil {
			t.Errorf("validateDatabaseName(%q) = %v, want it accepted", name, err)
		}
	}

	refused := map[string]string{
		"":                      "empty",
		"1app":                  "a leading digit",
		"app.data":              "a dot, which separates a namespace",
		"app$data":              "a dollar sign, which reads as an operator",
		"app data":              "a space",
		"app/data":              "a slash",
		`app\data`:              "a backslash",
		`app"data`:              "a quote",
		"app\x00":               "a null byte",
		strings.Repeat("a", 64): "longer than MongoDB stores",
		"café":                  "not ascii",
	}
	for name, why := range refused {
		if err := validateDatabaseName(name); err == nil {
			t.Errorf("validateDatabaseName(%q) was accepted; it has %s", name, why)
		}
	}
}

func TestValidateCollectionName(t *testing.T) {
	// A dot is ordinary in a collection name — the sub-collection convention uses
	// it — even though it is not allowed in a database name.
	accepted := []string{"users", "logs.2026", "_staging", "audit-log"}
	for _, name := range accepted {
		if err := validateCollectionName(name); err != nil {
			t.Errorf("validateCollectionName(%q) = %v, want it accepted", name, err)
		}
	}

	refused := map[string]string{
		"":                 "empty",
		"users$":           "a dollar sign",
		"1users":           "a leading digit",
		"system.users":     "the reserved prefix",
		"System.Users":     "the reserved prefix in mixed case",
		"SYSTEM.profile":   "the reserved prefix in upper case",
		"users\x00":        "a null byte",
		"users collection": "a space",
	}
	for name, why := range refused {
		if err := validateCollectionName(name); err == nil {
			t.Errorf("validateCollectionName(%q) was accepted; it has %s", name, why)
		}
	}
}

func TestValidateUserName(t *testing.T) {
	// An address is a common user name, so dots and at signs are allowed here.
	accepted := []string{"app", "app_user", "reporting.reader", "operator@example.invalid"}
	for _, name := range accepted {
		if err := validateUserName(name); err != nil {
			t.Errorf("validateUserName(%q) = %v, want it accepted", name, err)
		}
	}

	refused := []string{"", "1user", "user$", "user name", "user\x00", strings.Repeat("u", 129)}
	for _, name := range refused {
		if err := validateUserName(name); err == nil {
			t.Errorf("validateUserName(%q) was accepted", name)
		}
	}
}
