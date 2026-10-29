#!/bin/sh
set -eu

# docker-entrypoint-test.sh verifies persistent online updates through the real
# container startup script. Disposable containers share a temporary data volume
# and use small version-printing programs, so image replacement, online updates,
# rollback, and argument forwarding are observable without database connections.

# The caller supplies an existing TokenRouter runtime image containing su-exec
# and the service user. Test containers cannot access the production data directory.
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_image=${TOKENROUTER_TEST_IMAGE:?Set TOKENROUTER_TEST_IMAGE to a built runtime image}
fixture=$(mktemp -d)
# Remove fixture programs and their persistent directory after all test containers
# have exited; each invocation uses --rm and owns no lasting Docker container.
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
mkdir -p "$fixture/data"
chmod 0755 "$fixture"

# write_binary creates a version-reporting executable for a disposable image mount.
write_binary() {
    printf '#!/bin/sh\nprintf "%s:%%s:%%s\\n" "$(id -u)" "$*"\n' "$1" > "$2"
    chmod 0755 "$2"
}

# start runs the production entrypoint against fixture storage and a bundled
# program supplied by the test. The explicit network isolation prevents startup
# fixtures from making application or external service requests.
start() {
    docker run --rm --network none \
        -v "$repo_root/deploy/docker-entrypoint.sh:/app/docker-entrypoint.sh:ro" \
        -v "$fixture/bundled:/app/tokenrouter:ro" \
        -v "$fixture/data:/app/data" \
        --entrypoint /app/docker-entrypoint.sh "$test_image" "$@"
}

# An empty data volume requires installation under the service identity. The
# executed program must receive the original server arguments after privilege reduction.
write_binary v1 "$fixture/bundled"
[ "$(start /app/tokenrouter --version)" = 'v1:1000:--version' ]
[ -x "$fixture/data/runtime/tokenrouter" ]

# An online update replaces the installed executable while the image stays fixed.
# Recreating a container must keep that updated executable and preserve legacy flag handling.
write_binary v2 "$fixture/data/runtime/online-update"
chown 1000:1000 "$fixture/data/runtime/online-update"
mv "$fixture/data/runtime/online-update" "$fixture/data/runtime/tokenrouter"
[ "$(start --version)" = 'v2:1000:--version' ]
[ "$(start /app/sub2api --version)" = 'v2:1000:--version' ]

# A different image supplies a new bundled program. Startup must install that
# program and retain the previously selected online version as the rollback binary.
write_binary v3 "$fixture/bundled"
[ "$(start /app/tokenrouter --version)" = 'v3:1000:--version' ]
[ "$(sh "$fixture/data/runtime/tokenrouter.backup" --version)" = 'v2:0:--version' ]

# The rollback operation restores the saved program under the same image marker.
# The following container startup must honor that restored choice instead of reinstalling v3.
mv "$fixture/data/runtime/tokenrouter.backup" "$fixture/data/runtime/tokenrouter"
[ "$(start /app/tokenrouter --version)" = 'v2:1000:--version' ]

# Administrative shell commands bypass binary installation. They must still run
# as the service identity and receive their exact arguments from Docker.
[ "$(start /bin/sh -c 'id -u')" = '1000' ]
printf 'docker entrypoint persistence tests passed\n'
