# Handoff — Next Session

## What's Done

### v1.0.1 — Port reclassification (manual top 35)
### v1.1.0 — Demo app (hpr-marine blueprint)
### v1.2.0 — MCP server endpoint
### v1.3.0 — Batch endpoints

### v1.0.2 — Data Overhaul (THIS SESSION)

**Shipping Lanes:**
- Fixed filter bug (`Type` not `type` in MapLibre layer)
- Added 122 feeder lane segments (84 Middle, 38 Minor) connecting major/large ports >100km from existing lanes
- Hai Phong, Mombasa, Calcutta, etc. all now have lane connectivity

**Port Coordinates:**
- Fixed 84 port coordinates total (city-center → terminal-level)
- Key fixes: Hai Phong, Shanghai, Guangzhou, Ningbo, Chiwan, Yingkou, Rotterdam, Antwerp, Houston, Bangkok, etc.

**Port Classification (TEU + CPPI 2025):**
- Created `port-data/teu_data.csv` — 106 top container ports with TEU volumes
- Extracted 408 LOCODEs from CPPI 2025 report (annex2.pdf) → `port-data/cppi2025_locodes.txt`
- Reclassified 84 ports by TEU, upgraded 175 ports via CPPI 2025
- Script: `port-data/reclassify.py`

**Flight Routes:**
- Downloaded Jonty/airline-route-data (weekly updated, 82k+ airline-route pairs)
- Added 16,332 new confirmed route pairs
- Removed 230 routes pointing to non-existent airports
- Total: 523,496 routes (521k after dedup by callsign)

**Airlines:**
- Removed 4,018 defunct airlines (no IATA, no routes, not in Jonty)
- Kept 2,134 airlines (530 confirmed active by Jonty)

**Sea Routes GeoJSON:**
- New endpoint: `GET /v1/sea-routes/geojson?from={port_name}`
- Returns GeoJSON FeatureCollection with LineString features (pre-resolved coords)
- Fuzzy name matching (space-removal fallback for names like "HAI PHONG" → "HAIPHONG, VIETNAM")

**Web App:**
- Draggable panels (rail, vlist, apiguide, pcard) with pin-to-position + localStorage persistence
- `#view-map` as positioning context (position:relative;flex:1)
- Fixed shipping lanes rendering
- drawArcs uses GeoJSON endpoint instead of slow client-side name matching

## Current State

| Dataset | Records | Status |
|---------|---------|--------|
| Aircraft | 566k | Good (adsbdb) |
| Routes | 523k | Refreshed (Jonty 2025) |
| Airlines | 2,134 | Cleaned (defunct removed) |
| Airports | 8,001 | Good (OurAirports) |
| Ships | 746k | Good (ITU 2025) |
| Seaports | 3,630 | Fixed coords + reclassified |
| Sea Distances | 23k pairs | Good (NGA PUB 151) |
| Shipping Lanes | 3 tiers + feeders | Fixed + extended |

## What's Next

See `DATA-OVERHAUL.md` for full plan. Remaining phases:
- Phase 2: Full port coord merge with tayljordan/ports (if better coords found)
- Phase 3: Automated weekly Jonty pull (GitHub Actions cron)
- Phase 4: `searoute` geometry for sea route lines (curved paths instead of straight)

## Files Modified This Session
- `main.go` — sea-routes/geojson endpoint, sea-routes/from fuzzy matching, loadSeaDistancePorts
- `static/css/style.css` — #view-map, drag-handle/pin-btn CSS, layout fix
- `static/js/app.js` — drawArcs (GeoJSON endpoint), shipping lanes filter fix, drag+pin system
- `seaports.csv` — 84 coord fixes + 259 reclassifications
- `routes.csv` — +16,332 routes, -230 invalid
- `airlines.csv` — -4,018 defunct
- `shipping_lanes.geojson` — +122 feeder segments
- `port-data/teu_data.csv` — TEU volume data (106 ports)
- `port-data/cppi2025_locodes.txt` — 408 CPPI 2025 LOCODEs
- `port-data/reclassify.py` — TEU + CPPI reclassification script
- `port-data/fix_coords.py` — coordinate fix script (top ports)
- `port-data/feeder_lanes.geojson` — supplementary lane segments
- `port-data/jonty_routes.json` — Jonty route data (22MB)
- `port-data/active_airlines.txt` — 530 confirmed active airline ICAO codes
- `DATA-OVERHAUL.md` — full data quality plan

## Repo & Infra
- **Working dir**: `/srv/hpradar/api-route-go`
- **Live**: `https://traffic.hpradar.com` (port 5737 → 8081)
- **GitHub**: `ngtrthanh/hpr-traffic-api`
- **Docker**: `docker compose up -d --build`
