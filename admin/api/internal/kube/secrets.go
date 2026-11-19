package kube

import (
	"context"
	"fmt"
	"slices"
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
// This is the panel's only way to create a Secret, and it is deliberately narrow.
// The type is always Opaque, and an existing Secret is left alone
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

// validateSecretName checks the name of a Secret this panel is about to create.
//
// Deliberately stricter than Kubernetes, which allows a Secret a DNS *subdomain*:
// a Secret this panel writes is one somebody has to type into a values file, and
// the label rule is the one that keeps those typeable.
//
// It applies only to naming a NEW Secret. Addressing one that already exists uses
// validateExistingSecretName — see the note there for why conflating the two was
// a bug rather than a simplification.
func validateSecretName(name string) error {
	if err := validateNamespace(name); err != nil {
		return fmt.Errorf("%w: %q is not a valid Secret name", ErrInvalidName, name)
	}
	return nil
}

// validateExistingSecretName checks the name of a Secret somebody else created.
//
// The subdomain rule, which is Kubernetes' real one. This is not a relaxation for
// its own sake: Helm names a release Secret `sh.helm.release.v1.<release>.v<n>`,
// which has dots in it, so holding an existing Secret to the label rule refused
// every one of them as "not a valid Secret name" — and the deny-list that is
// supposed to protect Helm's release history could never be reached to say so.
//
// Two rules because there are two questions. What may this panel call a Secret it
// is creating is a matter of taste, and the strict answer is the good one. What
// may this panel be asked about is a matter of fact, and the only correct answer
// is whatever Kubernetes permits.
func validateExistingSecretName(name string) error {
	if err := validateName(name, "secret"); err != nil {
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
		if err := validateSecretKey(key); err != nil {
			return err
		}
		if value == "" {
			return fmt.Errorf("%w: %q has no value", ErrInvalidName, key)
		}
	}
	return nil
}

// validateSecretKey checks one data key against the characters and length
// Kubernetes accepts.
func validateSecretKey(key string) error {
	if key == "" || len(key) > maxSecretKeyLength || strings.Trim(key, secretKeyCharset) != "" {
		return fmt.Errorf("%w: %q is not a valid Secret key", ErrInvalidName, key)
	}
	return nil
}

// secretKeyCharset is every character Kubernetes permits in a data key.
const secretKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."

// ListSecrets reports the Secrets in a namespace, without any of their contents.
//
// The projection to SecretSummary happens in the repository, where the live
// objects are, so no value ever reaches this layer at all. That is deliberate: a
// rule enforced by what the service can see is harder to undo by accident than
// one enforced by remembering not to serialise a field.
//
// A protected namespace still lists. Protection is about writing, and the panel's
// own namespace holding the credentials it runs on is exactly the thing an
// operator wants to see and exactly the thing they must not delete — so the rows
// come back with Removable false and the reason, the same treatment the namespace
// list gives a namespace it will not delete.
func (s *Service) ListSecrets(ctx context.Context, namespace string) ([]SecretSummary, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}

	live, err := s.repo.ReadNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	if !s.policy.Managed(namespace, live.Labels) {
		return nil, fmt.Errorf("%w: %s", ErrNotManaged, namespace)
	}

	secrets, err := s.repo.ListSecrets(ctx, namespace)
	if err != nil {
		return nil, err
	}

	protected, protectedReason := s.policy.Protected(namespace, live.Labels)
	for i := range secrets {
		secrets[i].Removable, secrets[i].Reason = removable(secrets[i], protected, protectedReason)
	}
	return secrets, nil
}

// removable decides whether a row gets a delete and an edit, and says why not.
//
// Pure, and separate from the listing, because the panel and the two write paths
// have to agree about it: a row that offers a button the API then refuses is
// worse than one that never offered it.
func removable(secret SecretSummary, protected bool, protectedReason string) (bool, string) {
	if reserved, reason := reservedSecret(secret.Type); reserved {
		return false, reason
	}
	if protected {
		return false, protectedReason
	}
	return true, ""
}

// DeleteSecret removes a Secret from a namespace.
//
// The guards are PutSecret's, plus one: the Secret's own type. A namespace being
// managed and unprotected is what makes the panel's grant apply here at all, and
// reservedSecret is what stops that grant reaching the objects inside it that
// belong to something else.
//
// The type comes from a read rather than from the caller. A request naming a
// Secret is not evidence about what it is, and the whole point of the deny-list
// is to hold against a caller who would rather it did not.
func (s *Service) DeleteSecret(ctx context.Context, namespace, name string) error {
	target, err := s.writableSecret(ctx, namespace, name)
	if err != nil {
		return err
	}
	// The Secret that was checked, not its name: the repository binds the delete
	// to the object this verdict was reached about.
	return s.repo.DeleteSecret(ctx, namespace, target)
}

// EditSecret changes which keys a Secret holds and what some of them say.
//
// Three lists, and the split is the feature rather than a detail of the payload.
// Replace names keys that must already be there, Add names keys that must not,
// and Remove names keys that must. A single "set these keys" map would be
// shorter and would lose the one guard that matters here: an operator rotating
// "pasword" meant "password", and a map that adds whatever it does not find would
// write a second key beside the credential they believe they replaced. Saying
// which of the three each key is means a typo is refused instead of stored.
//
// The merge is done in the repository against the object it just read, so the
// keys not named keep their values and this layer never holds them. Note that
// this is an update, not a patch: the object written is the live one with some
// keys changed, so a key still has exactly one value and a Secret can never end
// up holding a new credential beside the one it replaced.
//
// An edit that would leave the Secret with no keys at all is refused. That is a
// delete, and the delete has its own confirmation for good reason.
func (s *Service) EditSecret(
	ctx context.Context, namespace, name string, edit SecretEdit,
) (SecretRef, error) {
	if err := validateSecretEdit(edit); err != nil {
		return SecretRef{}, err
	}

	live, err := s.writableSecret(ctx, namespace, name)
	if err != nil {
		return SecretRef{}, err
	}
	if live.Immutable {
		return SecretRef{}, fmt.Errorf("%w: %s is immutable", ErrReserved, name)
	}
	keys, err := editedKeys(name, live.Keys, edit)
	if err != nil {
		return SecretRef{}, err
	}

	if err := s.repo.EditSecretKeys(ctx, namespace, name, edit); err != nil {
		return SecretRef{}, err
	}
	return SecretRef{Namespace: namespace, Name: name, Keys: keys}, nil
}

// validateSecretEdit checks an edit's shape, before anything is read.
//
// Every key named anywhere must be one Kubernetes would accept, and every value
// must be non-empty for the reason validateSecretData gives. A key named twice —
// in two lists, or twice in Remove — is refused rather than resolved, because
// there is no order of applying it that is obviously what was meant.
func validateSecretEdit(edit SecretEdit) error {
	if len(edit.Replace)+len(edit.Add)+len(edit.Remove) == 0 {
		return fmt.Errorf("%w: an edit that changes nothing", ErrInvalidName)
	}

	seen := map[string]bool{}
	claim := func(key string) error {
		if err := validateSecretKey(key); err != nil {
			return err
		}
		if seen[key] {
			return fmt.Errorf("%w: %q is named more than once", ErrInvalidName, key)
		}
		seen[key] = true
		return nil
	}

	for _, values := range []map[string]string{edit.Replace, edit.Add} {
		for key, value := range values {
			if err := claim(key); err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("%w: %q has no value", ErrInvalidName, key)
			}
		}
	}
	for _, key := range edit.Remove {
		if err := claim(key); err != nil {
			return err
		}
	}
	return nil
}

// editedKeys checks an edit against the keys a Secret has and returns the keys
// it will have afterwards, sorted.
//
// Pure, and shared with the repository, which repeats it against the object it
// is about to write — see editable.
func editedKeys(name string, have []string, edit SecretEdit) ([]string, error) {
	for key := range edit.Replace {
		if !slices.Contains(have, key) {
			return nil, fmt.Errorf(
				"%w: %s has no key %q to replace — add it instead", ErrInvalidName, name, key)
		}
	}
	for _, key := range edit.Remove {
		if !slices.Contains(have, key) {
			return nil, fmt.Errorf("%w: %s has no key %q to remove", ErrInvalidName, name, key)
		}
	}
	for key := range edit.Add {
		if slices.Contains(have, key) {
			return nil, fmt.Errorf(
				"%w: %s already has a key %q — replace it instead", ErrAlreadyExists, name, key)
		}
	}

	keys := make([]string, 0, len(have)+len(edit.Add))
	for _, key := range have {
		if !slices.Contains(edit.Remove, key) {
			keys = append(keys, key)
		}
	}
	for key := range edit.Add {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf(
			"%w: removing every key from %s would leave it empty — delete the Secret instead",
			ErrInvalidName, name)
	}
	sort.Strings(keys)
	return keys, nil
}

// writableSecret runs every check the two write paths share and hands back the
// live Secret, so neither of them can be written with one of them missing.
func (s *Service) writableSecret(ctx context.Context, namespace, name string) (SecretSummary, error) {
	if err := validateNamespace(namespace); err != nil {
		return SecretSummary{}, err
	}
	if err := validateExistingSecretName(name); err != nil {
		return SecretSummary{}, err
	}

	live, err := s.repo.ReadNamespace(ctx, namespace)
	if err != nil {
		return SecretSummary{}, err
	}
	if protected, reason := s.policy.Protected(namespace, live.Labels); protected {
		return SecretSummary{}, fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}
	if !s.policy.Managed(namespace, live.Labels) {
		return SecretSummary{}, fmt.Errorf("%w: %s", ErrNotManaged, namespace)
	}

	secret, err := s.repo.ReadSecret(ctx, namespace, name)
	if err != nil {
		return SecretSummary{}, err
	}
	if reserved, reason := reservedSecret(secret.Type); reserved {
		return SecretSummary{}, fmt.Errorf("%w: %s is %s", ErrReserved, name, reason)
	}
	return secret, nil
}

// reservedSecret names the Secret types this panel will not touch, and says why
// in words a refusal can carry.
//
// Helm's release storage is the one that matters: a release's every revision —
// the chart, the values, the rendered manifests — is a Secret in the namespace it
// was installed into. Deleting one deletes history that nothing else has a copy
// of, and the panel reads those Secrets to answer every Helm question it is
// asked, so it would be destroying its own source of truth.
//
// A ServiceAccount token is here for a different reason: it is not damage that
// lasts. Kubernetes reissues one, so removing it breaks an identity for as long
// as it takes to notice and no longer. It is on the list because there is no
// version of "manage a namespace's credentials" that means this, and an operator
// who wanted it would reach for kubectl.
//
// Everything else — TLS material, registry pull credentials, an Opaque Secret a
// chart created — is deliberately not here. Those are things an operator does
// legitimately replace, and a deny-list long enough to cover every way to break a
// namespace would be a deny-list on managing it at all.
func reservedSecret(secretType string) (bool, string) {
	switch secretType {
	case helmReleaseSecretType:
		return true, "Helm's release storage"
	case serviceAccountTokenSecretType:
		return true, "a ServiceAccount token"
	default:
		return false, ""
	}
}

// The Secret types reservedSecret refuses, spelled as Kubernetes and Helm write
// them. Helm's is a literal rather than a constant from the Helm SDK: the value
// is part of Helm's storage format and this package does not import Helm.
//
// are the values being refused, and inlining them would only move the report.
//
//nolint:gosec // G101: these are Kubernetes type names, not credentials — they
const (
	helmReleaseSecretType         = "helm.sh/release.v1"
	serviceAccountTokenSecretType = "kubernetes.io/service-account-token"
)
