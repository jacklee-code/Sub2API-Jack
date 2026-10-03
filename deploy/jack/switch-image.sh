#!/usr/bin/env bash
# Run in the existing Docker Compose deployment directory. No Docker socket is
# exposed to the application. This also explicitly updates the persistent binary.
set -euo pipefail
version=${1:?Usage: switch-image.sh X.Y.Z-jack.N}
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-jack\.[1-9][0-9]*$ ]] || { echo 'Invalid Jack version' >&2; exit 2; }
image="ghcr.io/jacklee-code/sub2api-jack:$version"
[[ -f docker-compose.yml || -f compose.yml || -f compose.yaml ]] || { echo 'Run this in the Compose deployment directory' >&2; exit 2; }
exec 9>.jack-image-update.lock
flock -n 9 || { echo 'Another update is running' >&2; exit 1; }
if [[ -e docker-compose.override.yml ]]; then
  python3 -c 'import json; assert json.load(open("docker-compose.override.yml")).get("x-jack-managed") is True' || { echo 'Existing override is not managed by Jack; merge the image setting explicitly first.' >&2; exit 1; }
fi
docker pull "$image"
container=$(docker compose ps -q sub2api)
[[ -n "$container" ]] || { echo 'The existing sub2api container is required for backup' >&2; exit 1; }
umask 077
backup_dir="$PWD/.jack-backups/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup_dir"
for file in docker-compose.yml compose.yml compose.yaml docker-compose.override.yml .env; do
  [[ ! -f "$file" ]] || cp -p "$file" "$backup_dir/"
done
docker inspect "$container" --format '{{.Image}}' > "$backup_dir/image-id"
binary=/app/sub2api
if docker exec "$container" test -f /app/data/jack-runtime/sub2api; then binary=/app/data/jack-runtime/sub2api; fi
docker cp "$container:$binary" "$backup_dir/sub2api"
docker exec "$container" "$binary" -version > "$backup_dir/version.txt" 2>&1
# Quiesce application writers before the final database/data snapshot.
docker compose stop -t 60 sub2api
recovery_hint() { echo "Update failed. Backup retained at $backup_dir. See docs/JACK-DEVELOPMENT.md for recovery; database restoration is never automatic." >&2; }
trap recovery_hint ERR
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$backup_dir/database.dump"
docker run --rm --network none --volumes-from "$container" --entrypoint /bin/sh "$image" -c 'tar -C /app/data -czf - .' > "$backup_dir/app-data.tar.gz"
docker run --rm --network none --volumes-from "$container" --entrypoint /bin/sh "$image" -c 'mkdir -p /app/data/jack-runtime; cp /app/sub2api /app/data/jack-runtime/sub2api.next; chmod 755 /app/data/jack-runtime/sub2api.next; chown -R sub2api:sub2api /app/data/jack-runtime; mv /app/data/jack-runtime/sub2api.next /app/data/jack-runtime/sub2api'
python3 - "$image" <<'PY'
import json, os, sys
p='docker-compose.override.yml'
data=json.load(open(p)) if os.path.exists(p) else {'x-jack-managed':True}
data.setdefault('services',{}).setdefault('sub2api',{})['image']=sys.argv[1]
with open(p+'.next','w') as f: json.dump(data,f,indent=2); f.write('\n')
os.replace(p+'.next',p)
PY
docker compose up -d --no-deps --force-recreate sub2api
for _ in $(seq 1 60); do
  id=$(docker compose ps -q sub2api)
  health=$(docker inspect "$id" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}')
  if [[ "$health" == healthy ]]; then
    docker exec "$id" /app/data/jack-runtime/sub2api -version
    echo "Updated to $version; backup: $backup_dir"
    exit 0
  fi
  sleep 2
done
echo 'New container did not become healthy' >&2
exit 1
