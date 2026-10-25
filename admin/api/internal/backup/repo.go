package backup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/eetr-ai/home-lab/admin/api/internal/restconfig"
)

// The keys of the Secret that holds the connected account.
//
// The backup pods read refreshTokenKey through a secretKeyRef, so renaming it is a
// change to the CronJobs this API already created, not only to this file.
const (
	refreshTokenKey = "refreshToken"
	accountKey      = "account"
)

// Repository keeps this slice's state in the cluster: the connected account in a
// Secret, and the schedules as CronJobs.
//
// There is no table behind any of it. The CronJob is the schedule and the Secret
// is the connection, so nothing here can disagree with what the cluster will
// actually run.
type Repository struct {
	clientset kubernetes.Interface
	// namespace is the panel's own. Every object this slice touches lives there.
	namespace string
	// tokenSecret names the Secret holding the grant. It comes from the chart,
	// because the RBAC Role names it too.
	tokenSecret string
}

// NewRepository builds the repository. It contacts nothing.
func NewRepository(namespace, tokenSecret string) (*Repository, error) {
	config, err := restconfig.New()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes client for backups: %w", err)
	}
	return &Repository{clientset: clientset, namespace: namespace, tokenSecret: tokenSecret}, nil
}

// ReadGrant returns the connected account, or ErrNotConnected.
func (r *Repository) ReadGrant(ctx context.Context) (Grant, error) {
	secret, err := r.clientset.CoreV1().Secrets(r.namespace).Get(ctx, r.tokenSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Grant{}, ErrNotConnected
	}
	if err != nil {
		return Grant{}, translate(err, "read the Google grant")
	}

	ret := Grant{
		RefreshToken: string(secret.Data[refreshTokenKey]),
		Account:      string(secret.Data[accountKey]),
	}
	// A Secret somebody emptied by hand is no connection at all, and saying it is
	// one would send every backup off to fail at Google.
	if ret.RefreshToken == "" {
		return Grant{}, ErrNotConnected
	}
	return ret, nil
}

// WriteGrant stores the connected account, replacing whichever was there.
//
// Create first and update on a conflict, rather than get-then-decide: the RBAC
// Role grants create on Secrets generally and update only on this one by name, and
// a create that loses a race is the one case update exists for.
func (r *Repository) WriteGrant(ctx context.Context, grant Grant) error {
	secrets := r.clientset.CoreV1().Secrets(r.namespace)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.tokenSecret,
			Namespace: r.namespace,
			Labels:    map[string]string{labelManagedBy: managedBy, labelName: tokenComponent},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			refreshTokenKey: []byte(grant.RefreshToken),
			accountKey:      []byte(grant.Account),
		},
	}

	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	return translate(err, "store the Google grant")
}

// DeleteGrant forgets the connected account. Forgetting one that is not there is
// not an error.
func (r *Repository) DeleteGrant(ctx context.Context) error {
	err := r.clientset.CoreV1().Secrets(r.namespace).Delete(ctx, r.tokenSecret, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return translate(err, "forget the Google grant")
}

// translate turns an API server refusal into this slice's own errors, so the
// handler never has to know what client-go is.
func translate(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	case apierrors.IsAlreadyExists(err):
		return fmt.Errorf("%w: %s", ErrAlreadyExists, what)
	case apierrors.IsInvalid(err):
		// The API server's sentence is the useful part — it names the field, and
		// for a schedule that is almost always the cron expression.
		return fmt.Errorf("%w: %s: %w", ErrInvalid, what, err)
	case apierrors.IsForbidden(err):
		return fmt.Errorf("%w: %s: %w", ErrForbidden, what, err)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}
