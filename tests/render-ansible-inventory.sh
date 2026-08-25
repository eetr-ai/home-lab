#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fixture_bin="$repo_root/tests/fixtures/inventory-bin"
test_root=$(mktemp -d)

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT

prepare_test_repo() {
  local target=$1

  mkdir -p \
    "$target/scripts" \
    "$target/terraform" \
    "$target/ansible/inventory"
  cp "$repo_root/scripts/render-ansible-inventory.sh" "$target/scripts/"
}

bridge_repo="$test_root/bridge"
nat_repo="$test_root/nat"
missing_address_repo="$test_root/missing-address"
prepare_test_repo "$bridge_repo"
prepare_test_repo "$nat_repo"
prepare_test_repo "$missing_address_repo"

PATH="$fixture_bin:$PATH" MOCK_NETWORK_MODE=bridge \
  "$bridge_repo/scripts/render-ansible-inventory.sh" --vm-user vm-admin

jq -e '
  .all.vars == {"ansible_user":"vm-admin"} and
  .all.children.kubernetes.children.control_plane.hosts."k8s-cp-1".ansible_host == "192.0.2.10" and
  .all.children.kubernetes.children.workers.hosts."k8s-worker-1".ansible_host == "192.0.2.11"
' "$bridge_repo/ansible/inventory/hosts.yml" >/dev/null

PATH="$fixture_bin:$PATH" MOCK_NETWORK_MODE=network \
  "$nat_repo/scripts/render-ansible-inventory.sh" \
  --vm-user vm-admin \
  --libvirt-host libvirt-host

jq -e '
  .all.vars.ansible_ssh_common_args == "-o ProxyJump=libvirt-host" and
  .all.children.kubernetes.children.control_plane.hosts."k8s-cp-1".ansible_host == "192.0.2.20" and
  .all.children.kubernetes.children.workers.hosts."k8s-worker-1".ansible_host == "192.0.2.21"
' "$nat_repo/ansible/inventory/hosts.yml" >/dev/null

if PATH="$fixture_bin:$PATH" MOCK_NETWORK_MODE=bridge-missing-address \
  "$missing_address_repo/scripts/render-ansible-inventory.sh" \
  --vm-user vm-admin >/dev/null 2>&1; then
  printf 'Bridge inventory unexpectedly accepted a missing node address\n' >&2
  exit 1
fi

printf 'Inventory rendering tests passed.\n'
