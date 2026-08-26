package kube

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// LogOptions is what a caller may ask for when reading a pod's log.
type LogOptions struct {
	// Container names which one to read. Empty means the pod's only container,
	// and is an error at the API server when there is more than one — which is the
	// right error, and more useful than one this could invent.
	Container string
	// Follow keeps the stream open and forwards new lines as they are written.
	Follow bool
	// Tail bounds how much history to send first. A pod up for weeks would
	// otherwise dump all of it before the first new line arrives.
	Tail int64
	// Previous reads the log of the container's last terminated instance, which is
	// where the reason for a CrashLoopBackOff actually lives — the running
	// instance has not failed yet.
	Previous bool
}

// PodLogs opens a pod's log as a stream.
//
// The caller owns the returned reader and must close it. Closing it, or
// cancelling ctx, tears the request down at the API server too — which is what
// stops a closed browser tab from holding a connection open indefinitely.
func (r *Repository) PodLogs(
	ctx context.Context, namespace, pod string, options LogOptions,
) (io.ReadCloser, error) {
	podLogOptions := &corev1.PodLogOptions{
		Container: options.Container,
		Follow:    options.Follow,
		Previous:  options.Previous,
	}
	if options.Tail > 0 {
		podLogOptions.TailLines = &options.Tail
	}

	// Through the stream client, which has no request deadline. The ordinary one
	// would cut this off after twenty seconds. See NewStreamClientset.
	stream, err := r.streamClient.CoreV1().Pods(namespace).
		GetLogs(pod, podLogOptions).Stream(ctx)
	if err != nil {
		// Stream issues the request and returns an error on any non-2xx, so a 403
		// or a missing pod arrives here — before a single byte has been written,
		// which is the only point at which a status code is still possible.
		return nil, translate(err, "stream pod logs")
	}
	return stream, nil
}
