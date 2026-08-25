#!/usr/bin/env bash
set -euo pipefail

: "${KUBECONFIG:?Set KUBECONFIG to the ignored administrator kubeconfig}"

for command_name in curl kubectl; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

namespace='platform-system'
validation_namespace='platform-validation'

cleanup() {
  kubectl delete namespace "$validation_namespace" \
    --ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

kubectl wait --for=condition=Available deployment/traefik \
  --namespace "$namespace" --timeout=5m
kubectl wait --for=condition=Available deployment/cert-manager \
  --namespace "$namespace" --timeout=5m
kubectl wait --for=condition=Available deployment/cert-manager-webhook \
  --namespace "$namespace" --timeout=5m
kubectl wait --for=condition=Available deployment/nfs-subdir-external-provisioner \
  --namespace "$namespace" --timeout=5m

if kubectl get deployment cloudflared --namespace "$namespace" >/dev/null 2>&1; then
  kubectl wait --for=condition=Available deployment/cloudflared \
    --namespace "$namespace" --timeout=5m
fi

service_type=$(kubectl get service traefik --namespace "$namespace" \
  -o jsonpath='{.spec.type}')
if [[ $service_type != ClusterIP ]]; then
  printf 'Traefik must remain ClusterIP; found %s\n' "$service_type" >&2
  exit 1
fi

kubectl wait --for=condition=Accepted gatewayclass/traefik --timeout=3m
kubectl wait --for=condition=Programmed gateway/home-lab \
  --namespace "$namespace" --timeout=3m
kubectl wait --for=condition=Ready clusterissuer/letsencrypt-staging \
  clusterissuer/letsencrypt-production --timeout=5m

default_class=$(kubectl get storageclass nfs-client \
  -o jsonpath='{.metadata.annotations.storageclass\.kubernetes\.io/is-default-class}')
reclaim_policy=$(kubectl get storageclass nfs-client \
  -o jsonpath='{.reclaimPolicy}')
if [[ $default_class != true || $reclaim_policy != Delete ]]; then
  printf 'nfs-client must be the default StorageClass with Delete reclaim policy\n' >&2
  exit 1
fi

kubectl create namespace "$validation_namespace" >/dev/null
kubectl apply --namespace "$validation_namespace" -f - >/dev/null <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-validation
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: nfs-client
  resources:
    requests:
      storage: 32Mi
---
apiVersion: v1
kind: Pod
metadata:
  name: nfs-validation
spec:
  restartPolicy: Never
  containers:
    - name: writer
      image: busybox:1.37.0
      command: ["sh", "-c", "printf platform-ok > /data/result && sleep 3600"]
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: nfs-validation
EOF

kubectl wait --for=jsonpath='{.status.phase}'=Bound \
  persistentvolumeclaim/nfs-validation \
  --namespace "$validation_namespace" --timeout=3m
kubectl wait --for=condition=Ready pod/nfs-validation \
  --namespace "$validation_namespace" --timeout=3m

if [[ $(kubectl exec --namespace "$validation_namespace" nfs-validation -- cat /data/result) != platform-ok ]]; then
  printf 'NFS validation data did not round-trip\n' >&2
  exit 1
fi

pv_name=$(kubectl get persistentvolumeclaim nfs-validation \
  --namespace "$validation_namespace" -o jsonpath='{.spec.volumeName}')
kubectl delete namespace "$validation_namespace" --wait=true >/dev/null
kubectl wait --for=delete "persistentvolume/$pv_name" --timeout=3m

if kubectl get certificate home-lab-wildcard-tls \
  --namespace "$namespace" >/dev/null 2>&1; then
  kubectl wait --for=condition=Ready certificate/home-lab-wildcard-tls \
    --namespace "$namespace" --timeout=10m
fi

if [[ -n ${PLATFORM_TEST_URL:-} ]]; then
  curl --fail --silent --show-error --location \
    --max-time 30 "$PLATFORM_TEST_URL" >/dev/null
fi

printf 'Gateway API, Traefik, cert-manager, NFS, cloudflared, and public route validation passed.\n'
