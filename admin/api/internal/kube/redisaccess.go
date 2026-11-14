package kube

import (
	"context"
	"fmt"
)

// LabelRedisAccess is the label the platform's Redis NetworkPolicy admits
// namespaces by. The platform chart names it in values; this is the value it
// ships with, and the one the install scripts apply.
const LabelRedisAccess = "home-lab.example/redis-access"

// redisAccessGranted is the value the NetworkPolicy matches on.
const redisAccessGranted = "true"

// GrantRedisAccess labels a namespace so the platform's Redis admits it.
//
// The label is authorization, not authentication: a pod in the namespace still
// needs a Redis user and its password to do anything once it is let through. That
// is why this is a separate step from creating a user rather than a side effect of
// it — an app needs both, and the two are decided by different things.
func (s *Service) GrantRedisAccess(ctx context.Context, name string) (Namespace, error) {
	value := redisAccessGranted
	return s.setRedisAccess(ctx, name, &value)
}

// RevokeRedisAccess takes the label off again.
//
// Connections already open are not cut: a NetworkPolicy is enforced on new
// connections, and Cilium leaves an established one alone. The app loses Redis
// the next time it reconnects, which is worth knowing before revoking it from
// something running.
func (s *Service) RevokeRedisAccess(ctx context.Context, name string) (Namespace, error) {
	return s.setRedisAccess(ctx, name, nil)
}

// setRedisAccess writes or removes the one label, and nothing else.
//
// The grant behind this is `patch` on namespaces, which RBAC cannot narrow to a
// single label — the same credential could set pod-security.kubernetes.io/enforce
// to privileged. What stops it is that this is the only code that patches a
// namespace, and the key it patches is a constant. Protected namespaces are
// refused outright: platform-system is Redis's own, and the panel's namespace
// losing the label would take the panel's sign-in down with it.
func (s *Service) setRedisAccess(ctx context.Context, name string, value *string) (Namespace, error) {
	if err := validateNamespace(name); err != nil {
		return Namespace{}, err
	}

	// Read live, because protection is half decided by a label and the listing
	// the operator is looking at may be older than it.
	namespace, err := s.repo.ReadNamespace(ctx, name)
	if err != nil {
		return Namespace{}, err
	}
	if protected, reason := s.policy.Protected(namespace.Name, namespace.Labels); protected {
		return Namespace{}, fmt.Errorf("%w: %s is %s", ErrProtected, name, reason)
	}

	labelled, err := s.repo.LabelNamespace(ctx, name, LabelRedisAccess, value)
	if err != nil {
		return Namespace{}, err
	}
	s.applyPolicy(&labelled)
	return labelled, nil
}
