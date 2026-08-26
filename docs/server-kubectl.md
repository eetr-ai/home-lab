# Optional kubectl client on the virtualization host

The operator laptop is the primary cluster administration endpoint. Use this
procedure only when the physical virtualization host must also administer the
cluster. The copied administrator kubeconfig grants cluster-wide privileges.

Install the same pinned kubectl release used by the Ansible inventory. Run on
the Ubuntu virtualization host:

```bash
sudo apt update
sudo apt install -y ca-certificates curl
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL \
  https://pkgs.k8s.io/core:/stable:/v1.36/deb/Release.key \
  | sudo tee /etc/apt/keyrings/kubernetes-apt-keyring.asc >/dev/null

printf '%s\n' \
  'Types: deb' \
  'URIs: https://pkgs.k8s.io/core:/stable:/v1.36/deb/' \
  'Suites: /' \
  'Signed-By: /etc/apt/keyrings/kubernetes-apt-keyring.asc' \
  | sudo tee /etc/apt/sources.list.d/kubernetes.sources >/dev/null

sudo apt update
sudo apt install -y kubectl=1.36.4-1.1
sudo apt-mark hold kubectl
install -d -m 0700 "$HOME/.kube"
```

From the operator laptop, copy the ignored artifact fetched by Ansible:

```bash
ssh LIBVIRT_USER@LIBVIRT_HOST \
  'install -d -m 0700 "$HOME/.kube" &&
   test ! -e "$HOME/.kube/config"'
scp ansible/artifacts/admin.conf \
  LIBVIRT_USER@LIBVIRT_HOST:.kube/config
ssh LIBVIRT_USER@LIBVIRT_HOST 'chmod 0600 "$HOME/.kube/config"'
```

The preflight deliberately stops if a default configuration already exists.
Back up and merge contexts with `kubectl config view --flatten` instead of
silently overwriting an existing client configuration.

The default path removes the need to export `KUBECONFIG`. Verify without
`sudo`, which would select root's home and configuration:

```bash
ssh LIBVIRT_USER@LIBVIRT_HOST
kubectl config current-context
kubectl get nodes -o wide
kubectl get pods --all-namespaces
```

Do not commit, log, or place the kubeconfig on shared storage. Keep mode `0600`
and remove the file if the host no longer needs cluster-administrator access.
