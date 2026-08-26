#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'Usage: %s --vm-user USER [--libvirt-host HOST] [--network NETWORK]\n' "$0" >&2
}

libvirt_host=''
network=''
vm_user=''

while (($# > 0)); do
  case "$1" in
    --libvirt-host)
      libvirt_host=${2:-}
      shift 2
      ;;
    --network)
      network=${2:-}
      shift 2
      ;;
    --vm-user)
      vm_user=${2:-}
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [[ -z $vm_user ]]; then
  usage
  exit 2
fi

for value in "$libvirt_host" "$network" "$vm_user"; do
  [[ -n $value ]] || continue
  if [[ ! $value =~ ^[A-Za-z0-9._-]+$ ]]; then
    printf 'Unsafe host, network, or user value: %s\n' "$value" >&2
    exit 2
  fi
done

for command_name in jq ssh terraform; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
done

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
terraform_dir="$repo_root/terraform"
inventory_path="$repo_root/ansible/inventory/hosts.yml"
nodes_json=$(terraform -chdir="$terraform_dir" output -json nodes)
network_attachment=$(terraform -chdir="$terraform_dir" output -json network_attachment)
network_mode=$(jq -r '.mode' <<<"$network_attachment")
network_source=$(jq -r '.source' <<<"$network_attachment")

case "$network_mode" in
  bridge)
    ;;
  network)
    if [[ -z $libvirt_host ]]; then
      printf '%s\n' '--libvirt-host is required for a libvirt network attachment' >&2
      exit 2
    fi
    [[ -n $network ]] || network=$network_source
    ;;
  *)
    printf 'Unsupported Terraform network attachment mode: %s\n' "$network_mode" >&2
    exit 1
    ;;
esac

inventory=$(jq -n \
  --arg vm_user "$vm_user" \
  '{
    all: {
      vars: {
        ansible_user: $vm_user
      },
      children: {
        kubernetes: {
          children: {
            control_plane: {hosts: {}},
            workers: {hosts: {}}
          }
        }
      }
    }
  }')

if [[ $network_mode == network ]]; then
  inventory=$(jq \
    --arg proxy_jump "-o ProxyJump=$libvirt_host" \
    '.all.vars.ansible_ssh_common_args = $proxy_jump' \
    <<<"$inventory")
fi

while IFS= read -r node; do
  name=$(jq -r '.key' <<<"$node")
  role=$(jq -r '.value.role' <<<"$node")
  mac=$(jq -r '.value.mac' <<<"$node")

  if [[ $network_mode == bridge ]]; then
    address=$(jq -r '.value.ipv4_address // empty' <<<"$node")
    if [[ -z $address ]]; then
      printf 'No router-reserved ipv4_address is declared for bridged node %s\n' \
        "$name" >&2
      exit 1
    fi
  else
    lease_output=$(ssh -n "$libvirt_host" \
      virsh -c qemu:///system net-dhcp-leases "$network" --mac "$mac")
    address=$(awk 'NR > 2 && $5 != "-" {sub(/\/.*/, "", $5); print $5; exit}' \
      <<<"$lease_output")

    if [[ -z $address ]]; then
      printf 'No active DHCP lease for %s (%s) on network %s\n' \
        "$name" "$mac" "$network" >&2
      exit 1
    fi
  fi

  case "$role" in
    control-plane) group=control_plane ;;
    worker) group=workers ;;
    *)
      printf 'Unsupported Terraform node role for %s: %s\n' "$name" "$role" >&2
      exit 1
      ;;
  esac

  inventory=$(jq \
    --arg group "$group" \
    --arg name "$name" \
    --arg address "$address" \
    '.all.children.kubernetes.children[$group].hosts[$name] = {ansible_host: $address}' \
    <<<"$inventory")
done < <(jq -c 'to_entries[]' <<<"$nodes_json")

mkdir -p "$(dirname "$inventory_path")"
umask 077
printf '%s\n' "$inventory" >"$inventory_path"
printf 'Wrote %s\n' "$inventory_path"
