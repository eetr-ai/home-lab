package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// maxSecretKeyLength is Kubernetes' bound on a data key. The key is a path
// segment when the Secret is mounted as a volume, which is where the limit comes
// from.
const maxSecretKeyLength = 253

// PutSecret writes an Opaque Secret into a namespace.
//
// It exists for one workflow: the panel creates a database role, and the chart
// that will use it wants the password in a Secret. Everything needed is known at
// that moment, and without this the operator runs `kubectl create secret` by hand
// — which is the step that used to leave a credential in a shell history.
//
// This is the panel's only write to a Secret, and it is deliberately narrow. The
// type is always Opaque, there is no delete, and an existing Secret is left alone
// unless the caller says to replace it: overwriting one is how a running release
// loses the credential it was started with, so it takes saying so.
//
// A protected namespace is refused for the same reason it cannot be deleted. The
// check is made here, before anything reaches the cluster, so a refused request
// has written nothing.
func (s *Service) PutSecret(ctx context.Context, namespace string, spec SecretSpec) (SecretRef, error) {
	if err := validateNamespace(namespace); err != nil {
		return SecretRef{}, err
	}
	if err := validateSecretName(spec.Name); err != nil {
		return SecretRef{}, err
	}
	if err := validateSecretData(spec.Data); err != nil {
		return SecretRef{}, err
	}

	// Read the live labels rather than trusting the caller's word for the
	// namespace: protection can be applied by labelling the object, and a rule
	// that only consults configuration would miss it.
	live, err := s.repo.ReadNamespace(ctx, namespace)
	if err != nil {
		return SecretRef{}, err
	}
	if protected, reason := s.policy.Protected(namespace, live.Labels); protected {
		return SecretRef{}, fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}

	// Managed namespaces only, and this is the check that makes the refusal
	// legible rather than a 403 out of the API server. The grant that permits this
	// write is the same one that lets the panel read a namespace's releases — see
	// rbac-deploy.yaml — so a namespace the panel does not manage is one where the
	// write would fail anyway, and failing here says why.
	if !s.policy.Managed(namespace, live.Labels) {
		return SecretRef{}, fmt.Errorf("%w: %s", ErrNotManaged, namespace)
	}

	// The panel's own mark, applied over any label the caller sent, so a Secret
	// this wrote is distinguishable from one an operator or a chart created.
	// Nothing reads it today; it is what makes the question answerable later.
	labels := map[string]string{}
	for key, value := range spec.Labels {
		labels[key] = value
	}
	labels[labelManagedBy] = managedByValue

	written := SecretSpec{Name: spec.Name, Data: spec.Data, Labels: labels}
	if spec.Overwrite {
		if err := s.repo.UpdateSecret(ctx, namespace, written); err != nil {
			return SecretRef{}, err
		}
		return secretRef(namespace, spec), nil
	}

	if err := s.repo.CreateSecret(ctx, namespace, written); err != nil {
		return SecretRef{}, err
	}
	return secretRef(namespace, spec), nil
}

// secretRef describes what was written without carrying any of it. The keys are
// sorted so the response does not vary between identical requests.
func secretRef(namespace string, spec SecretSpec) SecretRef {
	keys := make([]string, 0, len(spec.Data))
	for key := range spec.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return SecretRef{Namespace: namespace, Name: spec.Name, Keys: keys}
}

// validateSecretName checks the name against the rule a namespace uses.
//
// Kubernetes allows a Secret a DNS *subdomain*, which is longer than this and may
// contain dots. The stricter rule is applied deliberately: reusing the check that
// already exists here means one place to be wrong about DNS labels rather than
// two, and a Secret this panel writes is one somebody has to type into a values
// file. If a chart ever needs a dotted name, the fix is a check of its own, not a
// relaxed one shared with namespaces.
func validateSecretName(name string) error {
	if err := validateNamespace(name); err != nil {
		return fmt.Errorf("%w: %q is not a valid Secret name", ErrInvalidName, name)
	}
	return nil
}

// validateSecretData checks the keys and refuses an empty payload.
//
// Kubernetes accepts alphanumerics, '-', '_' and '.' in a data key and nothing
// else. Checking here rather than letting the API server refuse keeps the failure
// a 400 with the offending key named, instead of a 500 wrapping a message about
// a field path.
//
// An empty value is refused rather than stored. A Secret whose password key holds
// "" is worse than no Secret at all: the workload starts and fails to
// authenticate, and nothing about that says the credential was never written.
func validateSecretData(data map[string]string) error {
	if len(data) == 0 {
		return fmt.Errorf("%w: a Secret with no data", ErrInvalidName)
	}
	for key, value := range data {
		if key == "" || len(key) > maxSecretKeyLength {
			return fmt.Errorf("%w: %q is not a valid Secret key", ErrInvalidName, key)
		}
		if strings.Trim(key, secretKeyCharset) != "" {
			return fmt.Errorf("%w: %q is not a valid Secret key", ErrInvalidName, key)
		}
		if value == "" {
			return fmt.Errorf("%w: %q has no value", ErrInvalidName, key)
		}
	}
	return nil
}

// secretKeyCharset is every character Kubernetes permits in a data key.
const secretKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."
