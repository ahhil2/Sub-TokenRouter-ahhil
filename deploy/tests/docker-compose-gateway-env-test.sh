#!/bin/sh
set -eu

# docker-compose-gateway-env-test.sh checks gateway settings in the Compose
# templates. Notification settings must reach the application when supplied and
# remain unresolved when absent, so mounted YAML can configure them. Compose
# configuration probes use fixture values and require no running containers.

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

# These fixture paths hold extracted settings and synthetic Compose environment
# files, with a shared cleanup trap covering normal completion and failures.
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/tokenrouter-gateway-env.XXXXXX")
gateway_variables="$fixture_dir/gateway-variables"

# cleanup removes this invocation's temporary configuration files and probe
# results. It runs on normal exit or interruption, after the fixture directory
# has been allocated successfully.
cleanup() {
  rm -rf "$fixture_dir"
}
trap cleanup EXIT HUP INT TERM

# Notification settings use optional entries because fixed defaults would
# override configured YAML, including a nonempty selected-user list.
optional_gateway_variables='GATEWAY_CODEX_CLIENT_NOTIFICATIONS GATEWAY_CODEX_CLIENT_NOTIFICATION_USER_IDS'

awk '
  /^GATEWAY_[A-Z0-9_]+=/ {
    separator = index($0, "=")
    print substr($0, 1, separator - 1) "\t" substr($0, separator + 1)
  }
' deploy/.env.example > "$gateway_variables"

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  tab=$(printf '\t')
  while IFS="$tab" read -r key value; do
    # The example shows larger connection pools for tuned deployments. Templates
    # retain backend defaults when operators have not supplied those tuning values.
    case "$key" in
      GATEWAY_MAX_CONNS_PER_HOST) value=1024 ;;
      GATEWAY_MAX_IDLE_CONNS) value=2560 ;;
      GATEWAY_MAX_IDLE_CONNS_PER_HOST) value=120 ;;
    esac

    expected=$(printf '      - %s=${%s:-%s}' "$key" "$key" "$value")
    expected_count=$(grep -Fxc "$expected" "$compose_file" || true)
    key_count=$(grep -Ec "^[[:space:]]*-[[:space:]]*${key}([[:space:]]*=.*)?[[:space:]]*$" "$compose_file" || true)
    if [ "$expected_count" -ne 1 ] || [ "$key_count" -ne 1 ]; then
      printf '%s must pass %s with the expected fallback exactly once\n' "$compose_file" "$key" >&2
      exit 1
    fi
  done < "$gateway_variables"

  for key in $optional_gateway_variables
  do
    expected_count=$(grep -Fxc "      - $key" "$compose_file" || true)
    key_count=$(grep -Ec "^[[:space:]]*-[[:space:]]*${key}([[:space:]]*=.*)?[[:space:]]*$" "$compose_file" || true)
    if [ "$expected_count" -ne 1 ] || [ "$key_count" -ne 1 ]; then
      printf '%s must pass optional %s exactly once without a default\n' "$compose_file" "$key" >&2
      exit 1
    fi
  done
done

for key in $optional_gateway_variables
do
  expected_count=$(grep -Fxc "      $key:" deploy/superahhil/compose.yaml || true)
  key_count=$(grep -Ec "^[[:space:]]*${key}:" deploy/superahhil/compose.yaml || true)
  if [ "$expected_count" -ne 1 ] || [ "$key_count" -ne 1 ]; then
    printf 'SuperAHHil Compose must pass optional %s exactly once without a default\n' "$key" >&2
    exit 1
  fi
  for sample in deploy/.env.example deploy/superahhil/.env.example
  do
    example_count=$(grep -Ec "^# ${key}=" "$sample" || true)
    configured_count=$(grep -Ec "^${key}=" "$sample" || true)
    if [ "$example_count" -ne 1 ] || [ "$configured_count" -ne 0 ]; then
      printf '%s must show %s as a commented example\n' "$sample" "$key" >&2
      exit 1
    fi
  done
done

# The static checks also run on machines without Compose. When the parser is
# installed, verify its resolved values using isolated shell and .env inputs.
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1; then
  python3 - "$repo_root" "$fixture_dir" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys

# All subprocesses receive synthetic credentials and explicit dotenv paths, so
# local operator settings cannot alter these configuration-resolution cases.
repo_root = Path(sys.argv[1])
fixture_dir = Path(sys.argv[2])
flag = "GATEWAY_CODEX_CLIENT_NOTIFICATIONS"
users = "GATEWAY_CODEX_CLIENT_NOTIFICATION_USER_IDS"
base_environment = {
    "PATH": os.environ["PATH"],
    "DATABASE_HOST": "postgres.example.invalid",
    "DATABASE_PASSWORD": "fixture-database",
    "POSTGRES_PASSWORD": "fixture-postgres",
    "REDIS_HOST": "redis.example.invalid",
    "REDIS_PASSWORD": "fixture-redis",
    "ADMIN_EMAIL": "fixture@example.invalid",
    "ADMIN_PASSWORD": "fixture-admin",
    "JWT_SECRET": "fixture-jwt",
    "TOTP_ENCRYPTION_KEY": "fixture-totp",
}
compose_files = [
    "deploy/docker-compose.yml",
    "deploy/docker-compose.local.yml",
    "deploy/docker-compose.standalone.yml",
    "deploy/docker-compose.dev.yml",
    "deploy/superahhil/compose.yaml",
]
cases = [
    ("unset", {}, {}, {flag: None, users: None}),
    ("enabled in dotenv", {flag: "true"}, {}, {flag: "true", users: None}),
    (
        "selected users in dotenv",
        {flag: "false", users: "101,102"}, {},
        {flag: "false", users: "101,102"},
    ),
    ("clear selected users", {users: ""}, {}, {flag: None, users: ""}),
    (
        "shell overrides dotenv",
        {flag: "true", users: "101,102"},
        {flag: "false", users: "303"},
        {flag: "false", users: "303"},
    ),
]
for case_name, dotenv, shell_values, expected in cases:
    dotenv_file = fixture_dir / "notification.env"
    dotenv_file.write_text("".join(f"{key}={value}\n" for key, value in dotenv.items()))
    for compose_file in compose_files:
        command = [
            "docker", "compose", "--env-file", str(dotenv_file),
            "-f", str(repo_root / compose_file), "config", "--format", "json",
        ]
        result = subprocess.run(
            command, env={**base_environment, **shell_values},
            text=True, capture_output=True,
        )
        if result.returncode:
            raise SystemExit(f"{compose_file}: {case_name}: {result.stderr.strip()}")
        environment = json.loads(result.stdout)["services"]["tokenrouter"]["environment"]
        actual = {key: environment.get(key) for key in expected}
        if actual != expected:
            raise SystemExit(f"{compose_file}: {case_name}: expected {expected}, got {actual}")
print(f"Compose notification environment probes passed ({len(cases) * len(compose_files)} cases)")
PY
else
  printf 'Compose notification parser probes skipped: Docker Compose and Python 3 are required\n'
fi

printf 'docker compose Gateway environment test passed\n'
