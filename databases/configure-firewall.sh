#!/usr/bin/env bash

set -euo pipefail

readonly CHAIN_NAME="HOME_LAB_DATABASES"
readonly NEXT_CHAIN_NAME="HOME_LAB_DB_NEXT"
readonly POSTGRES_PORT="5432"
readonly MONGO_PORT="27017"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="${script_dir}/.env"
action="${1:-}"

usage() {
  cat <<'EOF'
Usage: sudo ./configure-firewall.sh apply [ENV_FILE]
       sudo ./configure-firewall.sh remove
       sudo ./configure-firewall.sh status

apply   Allow only DATABASE_ALLOWED_CLIENTS to reach the database ports on
        DATABASE_BRIDGE_INTERFACE and DATABASE_LAN_ADDRESS.
remove  Remove only the HOME_LAB_DATABASES firewall chain and its jump.
status  Display the managed chain and its DOCKER-USER hook.
EOF
}

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

require_root() {
  if ((EUID != 0)); then
    fail "this action must run as root"
  fi
}

read_env_value() {
  local key="$1"
  local line

  line="$(grep -E "^${key}=" "$env_file" | tail -n 1 || true)"
  [[ -n "$line" ]] || fail "${key} is missing from ${env_file}"
  printf '%s' "${line#*=}"
}

validate_ipv4() {
  local address="$1"
  local octet
  local -a octets

  [[ "$address" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
  IFS='.' read -r -a octets <<<"$address"
  ((${#octets[@]} == 4)) || return 1

  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^[0-9]{1,3}$ ]] || return 1
    ((10#$octet <= 255)) || return 1
  done
}

delete_all_jumps() {
  local target_chain="$1"

  while iptables -w -C DOCKER-USER -j "$target_chain" 2>/dev/null; do
    iptables -w -D DOCKER-USER -j "$target_chain"
  done
}

delete_owned_chain() {
  local target_chain="$1"

  delete_all_jumps "$target_chain"
  if iptables -w -n -L "$target_chain" >/dev/null 2>&1; then
    iptables -w -F "$target_chain"
    iptables -w -X "$target_chain"
  fi
}

apply_rules() {
  local bridge_interface
  local lan_address
  local allowed_clients_value
  local client
  local existing
  local -a allowed_clients
  local -a seen_clients=()

  [[ -f "$env_file" ]] || fail "environment file not found: ${env_file}"
  bridge_interface="$(read_env_value DATABASE_BRIDGE_INTERFACE)"
  lan_address="$(read_env_value DATABASE_LAN_ADDRESS)"
  allowed_clients_value="$(read_env_value DATABASE_ALLOWED_CLIENTS)"

  [[ "$bridge_interface" =~ ^[a-zA-Z0-9_.:-]+$ ]] ||
    fail "DATABASE_BRIDGE_INTERFACE is invalid"
  [[ -e "/sys/class/net/${bridge_interface}" ]] ||
    fail "network interface does not exist: ${bridge_interface}"
  validate_ipv4 "$lan_address" || fail "DATABASE_LAN_ADDRESS must be an IPv4 address"

  IFS=',' read -r -a allowed_clients <<<"$allowed_clients_value"
  ((${#allowed_clients[@]} == 3)) ||
    fail "DATABASE_ALLOWED_CLIENTS must contain exactly three comma-separated IPv4 addresses"

  for client in "${allowed_clients[@]}"; do
    validate_ipv4 "$client" || fail "invalid client IPv4 address: ${client}"
    [[ "$client" != "$lan_address" ]] || fail "a client address matches DATABASE_LAN_ADDRESS"
    for existing in "${seen_clients[@]}"; do
      [[ "$client" != "$existing" ]] || fail "duplicate client IPv4 address: ${client}"
    done
    seen_clients+=("$client")
  done

  iptables -w -n -L DOCKER-USER >/dev/null 2>&1 ||
    fail "DOCKER-USER does not exist; start Docker before applying these rules"

  delete_owned_chain "$NEXT_CHAIN_NAME"
  iptables -w -N "$NEXT_CHAIN_NAME"

  for client in "${allowed_clients[@]}"; do
    iptables -w -A "$NEXT_CHAIN_NAME" \
      -i "$bridge_interface" -s "$client" -p tcp \
      -m conntrack --ctorigdst "$lan_address" --ctorigdstport "$POSTGRES_PORT" \
      -j RETURN
    iptables -w -A "$NEXT_CHAIN_NAME" \
      -i "$bridge_interface" -s "$client" -p tcp \
      -m conntrack --ctorigdst "$lan_address" --ctorigdstport "$MONGO_PORT" \
      -j RETURN
  done

  iptables -w -A "$NEXT_CHAIN_NAME" \
    -i "$bridge_interface" -p tcp \
    -m conntrack --ctorigdst "$lan_address" --ctorigdstport "$POSTGRES_PORT" \
    -j DROP
  iptables -w -A "$NEXT_CHAIN_NAME" \
    -i "$bridge_interface" -p tcp \
    -m conntrack --ctorigdst "$lan_address" --ctorigdstport "$MONGO_PORT" \
    -j DROP
  iptables -w -A "$NEXT_CHAIN_NAME" -j RETURN

  iptables -w -I DOCKER-USER 1 -j "$NEXT_CHAIN_NAME"
  delete_owned_chain "$CHAIN_NAME"
  iptables -w -E "$NEXT_CHAIN_NAME" "$CHAIN_NAME"

  printf 'Applied %s for %s on %s.\n' "$CHAIN_NAME" "$lan_address" "$bridge_interface"
  printf 'Run sudo netfilter-persistent save after verifying access.\n'
}

remove_rules() {
  delete_owned_chain "$NEXT_CHAIN_NAME"
  delete_owned_chain "$CHAIN_NAME"
  printf 'Removed the managed database firewall rules.\n'
  printf 'Run sudo netfilter-persistent save to persist the removal.\n'
}

show_status() {
  printf 'DOCKER-USER hook:\n'
  iptables -w -S DOCKER-USER | grep -F "$CHAIN_NAME" || printf 'No managed hook found.\n'
  printf '\nManaged chain:\n'
  if iptables -w -n -L "$CHAIN_NAME" --line-numbers; then
    return 0
  fi
  printf 'No managed chain found.\n'
}

case "$action" in
  apply)
    require_root
    env_file="${2:-$env_file}"
    command -v iptables >/dev/null 2>&1 || fail "iptables is not installed"
    apply_rules
    ;;
  remove)
    require_root
    command -v iptables >/dev/null 2>&1 || fail "iptables is not installed"
    remove_rules
    ;;
  status)
    require_root
    command -v iptables >/dev/null 2>&1 || fail "iptables is not installed"
    show_status
    ;;
  *)
    usage
    exit 2
    ;;
esac
