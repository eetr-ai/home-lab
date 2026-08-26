package kube

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReadStorage returns the cluster's persistent storage from both ends.
//
// Claims and volumes together, because they answer different questions: the
// claims say what workloads asked for, and the volumes say what exists —
// including a Released one still holding data that nothing is using.
func (r *Repository) ReadStorage(ctx context.Context) (Storage, error) {
	claims, err := r.listVolumeClaims(ctx)
	if err != nil {
		return Storage{}, err
	}
	volumes, err := r.listVolumes(ctx)
	if err != nil {
		return Storage{}, err
	}
	return Storage{Claims: claims, Volumes: volumes}, nil
}

// listVolumeClaims returns every PersistentVolumeClaim in the cluster.
func (r *Repository) listVolumeClaims(ctx context.Context) ([]VolumeClaim, error) {
	list, err := r.client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list persistent volume claims")
	}

	claims := make([]VolumeClaim, 0, len(list.Items))
	for i := range list.Items {
		claims = append(claims, summarizeClaim(&list.Items[i]))
	}
	sort.Slice(claims, func(a, b int) bool {
		if claims[a].Namespace != claims[b].Namespace {
			return claims[a].Namespace < claims[b].Namespace
		}
		return claims[a].Name < claims[b].Name
	})
	return claims, nil
}

// summarizeClaim builds the reported shape from one claim.
func summarizeClaim(claim *corev1.PersistentVolumeClaim) VolumeClaim {
	return VolumeClaim{
		Name:      claim.Name,
		Namespace: claim.Namespace,
		Status:    string(claim.Status.Phase),
		// The request lives in the spec and the granted size in the status. A
		// pending claim has the first and not the second, which is exactly the
		// difference worth showing.
		RequestedBytes: storageBytes(claim.Spec.Resources.Requests),
		CapacityBytes:  storageBytes(claim.Status.Capacity),
		StorageClass:   deref(claim.Spec.StorageClassName),
		VolumeName:     claim.Spec.VolumeName,
		AccessModes:    accessModes(claim.Spec.AccessModes),
		CreatedAt:      claim.CreationTimestamp.Time,
	}
}

// listVolumes returns every PersistentVolume in the cluster.
func (r *Repository) listVolumes(ctx context.Context) ([]Volume, error) {
	list, err := r.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list persistent volumes")
	}

	volumes := make([]Volume, 0, len(list.Items))
	for i := range list.Items {
		volumes = append(volumes, summarizeVolume(&list.Items[i]))
	}
	sort.Slice(volumes, func(a, b int) bool { return volumes[a].Name < volumes[b].Name })
	return volumes, nil
}

// summarizeVolume builds the reported shape from one volume.
func summarizeVolume(volume *corev1.PersistentVolume) Volume {
	claim := ""
	if reference := volume.Spec.ClaimRef; reference != nil {
		claim = fmt.Sprintf("%s/%s", reference.Namespace, reference.Name)
	}
	return Volume{
		Name:          volume.Name,
		Status:        string(volume.Status.Phase),
		CapacityBytes: storageBytes(volume.Spec.Capacity),
		StorageClass:  volume.Spec.StorageClassName,
		Claim:         claim,
		ReclaimPolicy: string(volume.Spec.PersistentVolumeReclaimPolicy),
		AccessModes:   accessModes(volume.Spec.AccessModes),
		CreatedAt:     volume.CreationTimestamp.Time,
	}
}

// storageBytes reads the storage quantity out of a resource list.
func storageBytes(list corev1.ResourceList) int64 {
	if storage, ok := list[corev1.ResourceStorage]; ok {
		return storage.Value()
	}
	return 0
}

// accessModes renders the modes as their short names, the way kubectl prints them.
func accessModes(modes []corev1.PersistentVolumeAccessMode) []string {
	short := make([]string, 0, len(modes))
	for _, mode := range modes {
		switch mode {
		case corev1.ReadWriteOnce:
			short = append(short, "RWO")
		case corev1.ReadOnlyMany:
			short = append(short, "ROX")
		case corev1.ReadWriteMany:
			short = append(short, "RWX")
		case corev1.ReadWriteOncePod:
			short = append(short, "RWOP")
		default:
			short = append(short, string(mode))
		}
	}
	return short
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
