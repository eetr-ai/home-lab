package helm

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValues(t *testing.T) {
	t.Run("an empty document is an empty map, not an error", func(t *testing.T) {
		for _, document := range []string{"", "   ", "\n\n", "# only a comment\n"} {
			values, err := parseValues(document)
			if err != nil {
				t.Fatalf("parsing %q: %v", document, err)
			}
			if len(values) != 0 {
				t.Errorf("parsing %q: want no values, got %v", document, values)
			}
		}
	})

	t.Run("a mapping parses", func(t *testing.T) {
		values, err := parseValues("replicaCount: 2\nui:\n  message: hello\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if values["replicaCount"] != float64(2) {
			t.Errorf("replicaCount: got %#v", values["replicaCount"])
		}
		ui, ok := values["ui"].(map[string]any)
		if !ok || ui["message"] != "hello" {
			t.Errorf("ui: got %#v", values["ui"])
		}
	})

	t.Run("broken YAML is refused, and the message names the line", func(t *testing.T) {
		_, err := parseValues("replicaCount: 2\n  bad: indentation\n")
		if !errors.Is(err, ErrInvalidValues) {
			t.Fatalf("want ErrInvalidValues, got %v", err)
		}
		if !strings.Contains(err.Error(), "line") {
			t.Errorf("the error should name the line, and says: %v", err)
		}
	})

	t.Run("a document that is not a mapping is refused", func(t *testing.T) {
		if _, err := parseValues("- one\n- two\n"); !errors.Is(err, ErrInvalidValues) {
			t.Errorf("a list should be refused, got %v", err)
		}
	})

	t.Run("values too large to store are refused", func(t *testing.T) {
		document := "big: " + strings.Repeat("x", maxValuesBytes)
		if _, err := parseValues(document); !errors.Is(err, ErrValuesTooLarge) {
			t.Errorf("want ErrValuesTooLarge, got %v", err)
		}
	})
}

// The merge is what a pipeline's overrides go through, so its rules are the ones
// a pipeline's behaviour depends on.
func TestMergeValues(t *testing.T) {
	base := map[string]any{
		"replicaCount": float64(2),
		"image":        map[string]any{"repository": "podinfo", "tag": "6.0.0"},
		"hosts":        []any{"a.example.com"},
		"ui":           map[string]any{"message": "hello"},
	}
	override := map[string]any{
		"image": map[string]any{"tag": "6.1.0"},
		"hosts": []any{"b.example.com"},
		"extra": true,
	}

	merged := mergeValues(base, override)

	image, _ := merged["image"].(map[string]any)
	if image["tag"] != "6.1.0" {
		t.Errorf("the override should replace the tag, got %#v", image["tag"])
	}
	// The whole point of a deep merge: a pipeline setting image.tag must not
	// erase image.repository, which is the operator's.
	if image["repository"] != "podinfo" {
		t.Errorf("the sibling key should survive, got %#v", image["repository"])
	}
	if merged["replicaCount"] != float64(2) {
		t.Errorf("an untouched key should survive, got %#v", merged["replicaCount"])
	}
	if merged["extra"] != true {
		t.Errorf("a new key should be added, got %#v", merged["extra"])
	}

	// Lists replace rather than concatenate: appending would mean a pipeline
	// setting one host adds a second instead of changing the first.
	hosts, _ := merged["hosts"].([]any)
	if len(hosts) != 1 || hosts[0] != "b.example.com" {
		t.Errorf("lists should replace, got %#v", merged["hosts"])
	}

	if ui, _ := merged["ui"].(map[string]any); ui["message"] != "hello" {
		t.Errorf("an untouched branch should survive, got %#v", merged["ui"])
	}
}

// A merged result must share no map with its inputs, or storing it and then
// editing it would rewrite the version it came from.
func TestMergeValuesDoesNotAliasItsInputs(t *testing.T) {
	base := map[string]any{"image": map[string]any{"tag": "1.0.0"}}
	merged := mergeValues(base, map[string]any{})

	mergedImage, _ := merged["image"].(map[string]any)
	mergedImage["tag"] = "changed"

	baseImage, _ := base["image"].(map[string]any)
	if baseImage["tag"] != "1.0.0" {
		t.Errorf("the base was mutated through the merged copy: %#v", baseImage)
	}
}

// Values written by a pipeline round-trip back to a document an operator can
// open, and it says where it came from.
func TestRenderValues(t *testing.T) {
	document, err := renderValues(map[string]any{"replicaCount": 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(document, "#") {
		t.Errorf("rendered values should carry the generated header, got %q", document)
	}

	values, err := parseValues(document)
	if err != nil {
		t.Fatalf("the rendered document should parse back: %v", err)
	}
	if values["replicaCount"] != float64(3) {
		t.Errorf("round trip lost the value: %#v", values)
	}

	empty, err := renderValues(map[string]any{})
	if err != nil || empty != "" {
		t.Errorf("no values should render as an empty document, got %q / %v", empty, err)
	}
}
