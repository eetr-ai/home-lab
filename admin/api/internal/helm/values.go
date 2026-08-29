package helm

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// maxValuesBytes bounds the values a request may carry.
//
// Values end up in a Secret, and a Secret is capped at roughly a megabyte by
// etcd. A request over that limit fails at the moment Helm writes the release,
// which is after the chart has already been applied to the cluster — so the
// refusal has to happen here, before anything has been done.
const maxValuesBytes = 256 * 1024

// generatedHeader marks values this API wrote rather than a person.
//
// A version created by a pipeline has no hand-written document behind it, and
// saying so in the file is kinder than letting an operator open the editor and
// wonder where their comments went.
const generatedHeader = "# Written by a pipeline; the comments in the previous version are not carried over.\n"

// parseValues turns a values document into the map Helm takes.
//
// An empty document is an empty map rather than an error: "no values" is a
// perfectly good thing to mean, and it is what the chart's own defaults are for.
func parseValues(document string) (map[string]any, error) {
	if err := checkValuesSize(document); err != nil {
		return nil, err
	}
	if strings.TrimSpace(document) == "" {
		return map[string]any{}, nil
	}

	var values map[string]any
	if err := yaml.Unmarshal([]byte(document), &values); err != nil {
		// The underlying error names the line, which is the only part of this an
		// operator can act on, so it is carried through to the response rather
		// than replaced with something tidier.
		return nil, fmt.Errorf("%w: %w", ErrInvalidValues, err)
	}
	if values == nil {
		values = map[string]any{}
	}
	return values, nil
}

// renderValues turns a map back into a document.
func renderValues(values map[string]any) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	encoded, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("%w: these values cannot be written back as YAML", ErrInvalidValues)
	}
	return generatedHeader + string(encoded), nil
}

// checkValuesSize refuses a document too large to be stored.
func checkValuesSize(document string) error {
	if len(document) > maxValuesBytes {
		return fmt.Errorf("%w: the values are %d bytes, and at most %d are accepted",
			ErrValuesTooLarge, len(document), maxValuesBytes)
	}
	return nil
}

// mergeValues lays one set of values over another, the way Helm merges a -f file
// under a --set.
//
// Maps are merged key by key at every depth; everything else replaces. Lists
// replace rather than concatenate, which is Helm's own rule and the only one that
// makes a pipeline predictable: appending would mean a pipeline that sets one
// ingress host adds a second rather than changing the first.
//
// Neither argument is modified. The result shares no map with either, so a
// caller cannot mutate stored values by editing what it got back.
func mergeValues(base, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		merged[key] = copyValue(value)
	}

	for key, value := range override {
		existing, found := merged[key]
		if !found {
			merged[key] = copyValue(value)
			continue
		}

		existingMap, existingIsMap := existing.(map[string]any)
		valueMap, valueIsMap := value.(map[string]any)
		if existingIsMap && valueIsMap {
			merged[key] = mergeValues(existingMap, valueMap)
			continue
		}
		merged[key] = copyValue(value)
	}
	return merged
}

// copyValue deep-copies the containers, so a merged result never aliases its
// inputs.
func copyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, inner := range typed {
			copied[key] = copyValue(inner)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for index, inner := range typed {
			copied[index] = copyValue(inner)
		}
		return copied
	default:
		return value
	}
}
