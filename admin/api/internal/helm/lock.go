package helm

import "sync"

// locks stops two operations on the same release starting at once in this
// process.
//
// It is NOT the real guard, and reading it as one would be a mistake. The API
// runs two replicas and this map is per-process, so two operators on two replicas
// can still race. What actually prevents that is Helm's own storage: an operation
// against a release already in a pending state is refused, because the previous
// revision was never marked done.
//
// This exists for the common case — a double-clicked button, a pipeline that
// retried — where the second attempt should be a clean 409 rather than whatever
// error string Helm produces for a release mid-flight.
type locks struct {
	mu   sync.Mutex
	held map[string]bool
}

func newLocks() *locks {
	return &locks{held: map[string]bool{}}
}

// acquire takes the lock for one release, and reports whether it got it.
func (l *locks) acquire(namespace, name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := namespace + "/" + name
	if l.held[key] {
		return false
	}
	l.held[key] = true
	return true
}

func (l *locks) release(namespace, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.held, namespace+"/"+name)
}
