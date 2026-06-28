#!/bin/bash
# Refresh all external data sources
set -e
cd "$(dirname "$0")/.."

echo "=== HPRadar Data Refresh ==="

# 1. Aircraft — Mictronics (weekly)
echo "[1/3] Aircraft (Mictronics)..."
wget -q https://github.com/Mictronics/aircraft-database/raw/main/icao24plus.zip -O /tmp/mic.zip
unzip -oq /tmp/mic.zip -d /tmp/mictronics
python3 scripts/merge-mictronics.py /tmp/mictronics/icao24plus.txt
rm -f /tmp/mic.zip

# 2. Airports — OurAirports (monthly)
echo "[2/3] Airports (OurAirports)..."
wget -q https://davidmegginson.github.io/ourairports-data/airports.csv -O /tmp/ourairports.csv
python3 -c "
import csv
out = []
with open('/tmp/ourairports.csv') as f:
    r = csv.DictReader(f)
    for row in r:
        if row['type'] in ('closed',): continue
        icao = row.get('gps_code') or row.get('ident','')
        if not icao or len(icao) != 4: continue
        iata = row.get('iata_code','')
        lat = row.get('latitude_deg','')
        lon = row.get('longitude_deg','')
        if not lat or not lon: continue
        sched = '1' if row.get('scheduled_service','no') == 'yes' else '0'
        out.append([icao, iata, row.get('name',''), row.get('municipality',''),
                    row.get('iso_country',''), lat, lon, row.get('type',''), sched])
with open('data/airports.csv','w',newline='') as f:
    w = csv.writer(f)
    w.writerow(['icao','iata','name','city','country_code','latitude','longitude','type','scheduled'])
    w.writerows(out)
print(f'  Airports: {len(out)}')
"
rm -f /tmp/ourairports.csv

# 3. Routes — Jonty (monthly)
echo "[3/3] Routes (if update-routes.sh exists)..."
if [ -f scripts/update-routes.sh ]; then
    bash scripts/update-routes.sh
else
    echo "  Skipped (no update-routes.sh)"
fi

echo ""
echo "=== Done. Rebuild to apply: docker compose up -d --build ==="
