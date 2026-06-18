#!/bin/bash
# Sync missing callsigns from live tracker into routes.csv
# Then rebuild containers to load new data

set -euo pipefail

DIR="/srv/hpradar/api-route-go"
TRACKER="https://skylink.hpradar.com/data/aircraft.json"
LOG="/var/log/sync-routes.log"

echo "$(date '+%Y-%m-%d %H:%M:%S') Starting route sync" >> "$LOG"

# Extract missing airline callsigns
MISSING=$(curl -s "$TRACKER" | python3 -c "
import json, sys, re
data = json.load(sys.stdin)
existing = set()
with open('$DIR/routes.csv') as f:
    next(f)
    for line in f:
        existing.add(line.split(',')[0])
for a in data['aircraft']:
    f = a.get('flight','').strip()
    if f and re.match(r'^[A-Z]{2,4}\d', f) and f not in existing:
        print(f)
" | sort -u)

COUNT=$(echo "$MISSING" | grep -c . || true)
if [ "$COUNT" -eq 0 ]; then
    echo "$(date '+%Y-%m-%d %H:%M:%S') No new callsigns" >> "$LOG"
    exit 0
fi

echo "$MISSING" > "$DIR/missing_callsigns.txt"
echo "$(date '+%Y-%m-%d %H:%M:%S') Looking up $COUNT callsigns" >> "$LOG"

cd "$DIR"
./update-tool -input missing_callsigns.txt -csv routes.csv -delay 100ms >> "$LOG" 2>&1

# Restart containers to load new data
docker compose -f /srv/lab/api-route-go/docker-compose.yml up -d --build >> "$LOG" 2>&1
docker compose -f /srv/hpradar/api-route-go/docker-compose.yml up -d --build >> "$LOG" 2>&1

echo "$(date '+%Y-%m-%d %H:%M:%S') Sync complete" >> "$LOG"
