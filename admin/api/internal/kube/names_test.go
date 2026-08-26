package kube

import (
	"strings"
	"testing"
)

func TestValidateNamespace(t *testing.T) {
	accepted := []string{"default", "kube-system", "platform-system", "a", "a1", strings.Repeat("a", 63)}
	for _, name := range accepted {
		if err := validateNamespace(name); err != nil {
			t.Errorf("validateNamespace(%q) = %v, want it accepted", name, err)
		}
	}

	refused := map[string]string{
		"":                      "empty",
		"Default":               "an upper-case letter",
		"kube_system":           "an underscore",
		"-leading":              "a leading hyphen",
		"trailing-":             "a trailing hyphen",
		"has space":             "a space",
		"a/b":                   "a slash, which would change the request path",
		"..":                    "dots",
		strings.Repeat("a", 64): "longer than a DNS label",
	}
	for name, why := range refused {
		if err := validateNamespace(name); err == nil {
			t.Errorf("validateNamespace(%q) was accepted; it has %s", name, why)
		}
	}
}
