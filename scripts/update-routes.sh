#!/bin/bash
# Weekly route update from Jonty/airline-route-data
# Run via cron: 0 3 * * 0 /srv/hpradar/api-route-go/scripts/update-routes.sh
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$DIR"

JONTY_URL="https://raw.githubusercontent.com/jpatokal/openflights/master/data/routes.dat"
TMP="/tmp/jonty_routes.dat"

echo "[$(date)] Fetching Jonty route data..."
curl -sL "$JONTY_URL" -o "$TMP"
LINES=$(wc -l < "$TMP")
echo "  Downloaded $LINES routes"

if [ "$LINES" -lt 10000 ]; then
  echo "  ERROR: Too few routes ($LINES), aborting"
  exit 1
fi

# Convert Jonty format to our routes.csv format
# Jonty columns: airline,airline_id,src_airport,src_id,dst_airport,dst_id,codeshare,stops,equipment
# Our format: callsign,airline_code,airport_codes
python3 -c "
import csv, sys

existing = set()
with open('routes.csv') as f:
    r = csv.reader(f)
    next(r)  # skip header
    for row in r:
        existing.add(row[0])  # callsign

added = 0
with open('$TMP') as f, open('routes.csv', 'a') as out:
    w = csv.writer(out)
    for line in f:
        parts = line.strip().split(',')
        if len(parts) < 7:
            continue
        airline = parts[0].strip()
        src = parts[2].strip()
        dst = parts[4].strip()
        if not airline or not src or not dst or len(src) != 4 or len(dst) != 4:
            continue
        # Generate synthetic callsign
        callsign = f'{airline}{src}{dst}'[:10]
        if callsign in existing:
            continue
        existing.add(callsign)
        w.writerow([callsign, airline, f'{src}-{dst}'])
        added += 1

print(f'  Added {added} new routes')
"

rm -f "$TMP"

# Rebuild container
echo "[$(date)] Rebuilding..."
docker compose up -d --build 2>&1 | tail -2
echo "[$(date)] Done"
