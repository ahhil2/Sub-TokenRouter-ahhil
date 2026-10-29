#!/bin/sh
set -eu

# docker-entrypoint.sh prepares persistent application storage and starts the
# selected command as the tokenrouter user. The normal server command runs from
# the data volume, where the online updater can stage downloads and replace the
# executable. Restarts and recreation with the same image reuse that installed
# program, including versions installed through the administration page.
#
# A checksum records the binary supplied by the container image. A changed image
# installs its binary before startup, retaining the previous executable for local
# rollback. The checksum is published after the executable replacement; interrupted
# installation is retried at the next start. One container owns each runtime directory.

if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Read-only configuration mounts can reject ownership changes while writable
    # data remains usable, so initialization proceeds and concrete writes report errors.
    chown -R tokenrouter:tokenrouter /app/data 2>/dev/null || true
    # Re-enter under the service identity before writing runtime programs so
    # subsequent update and rollback requests have permission to manage their files.
    exec su-exec tokenrouter "$0" "$@"
fi

# Older Compose commands pass server flags directly. Supply the bundled program
# path so those commands receive the same persistent installation behavior.
if [ "$#" -eq 0 ]; then
    set -- /app/tokenrouter
elif [ "${1#-}" != "$1" ]; then
    set -- /app/tokenrouter "$@"
fi

case "$1" in
    /app/tokenrouter|/app/sub2api)
        # The checksum identifies image changes separately from online updates.
        # Reusing an unchanged image must keep the program selected by the administrator.
        runtime_dir=/app/data/runtime
        runtime_binary="$runtime_dir/tokenrouter"
        image_marker="$runtime_dir/image.sha256"
        bundled_checksum=$(sha256sum /app/tokenrouter | cut -d ' ' -f 1)
        installed_checksum=''
        if [ -f "$image_marker" ]; then
            installed_checksum=$(cat "$image_marker")
        fi
        mkdir -p "$runtime_dir"
        if [ ! -x "$runtime_binary" ] || [ "$installed_checksum" != "$bundled_checksum" ]; then
            # Stage complete files on the volume before replacing the executable.
            # An interrupted copy leaves the active program intact for another attempt.
            staged_binary=$(mktemp "$runtime_dir/.bootstrap-binary.XXXXXX")
            staged_marker=$(mktemp "$runtime_dir/.bootstrap-marker.XXXXXX")
            trap 'rm -f "$staged_binary" "$staged_marker"' EXIT HUP INT TERM
            cp /app/tokenrouter "$staged_binary"
            chmod 0755 "$staged_binary"
            printf '%s\n' "$bundled_checksum" > "$staged_marker"
            if [ -f "$runtime_binary" ]; then
                cp "$runtime_binary" "$runtime_binary.backup"
            fi
            # Publish the executable before its image identity; a failure between
            # these moves repeats installation on startup rather than accepting an incomplete copy.
            mv -f "$staged_binary" "$runtime_binary"
            mv -f "$staged_marker" "$image_marker"
            trap - EXIT HUP INT TERM
        fi
        shift
        set -- "$runtime_binary" "$@"
        ;;
esac

exec "$@"
