# Data Overhaul Plan — HPR Traffic API

## Problem Statement

Current datasets have multiple quality issues:
- Port coordinates placed at city centers (e.g., Hai Phong at 20.92,106.68 — should be 20.82,106.78 at Lach Huyen terminal)
- ~187 ports with suspiciously rounded coordinates (1-decimal precision = city-level)
- Outdated flight routes (adsbdb community data, sporadic updates)
- Airline reference from VRS standing-data (last meaningful update 2022)
- Shipping lanes GeoJSON had wrong property name (fixed: `Type` not `type`)
- Sea distances from NGA PUB 151 (1990s data, many ports renamed/relocated)

---

## Domain: Maritime (Ports)

### Current State
- 3,630 ports from NGA World Port Index (Pub 150) + UPPLY enrichment
- Coordinates from NGA (many are city-center, not terminal-level)
- Port size from NGA (manually corrected top 100 by TEU + CPPI 2025)

### Recommended Sources (Priority Order)

| Source | Records | Quality | Notes |
|--------|---------|---------|-------|
| **tayljordan/ports** (GitHub) | 3,898 | ★★★★★ | Terminal-level lat/lon from Google Maps, Jan 2025. Best coordinates. |
| **UPPLY Open Data** | ~4,000 | ★★★★ | LOCODEs, zones, updated regularly |
| **NGA WPI Pub 150** | 3,700 | ★★★ | Authoritative but coords are often port-center, not terminal |
| **MarineTraffic/VesselFinder** | N/A | ★★★★★ | Best coords but not freely available bulk |

### Action Plan

1. **Replace coordinates** — Use `tayljordan/ports` as primary coord source (terminal-level from Google Maps)
2. **Keep NGA metadata** — depth, restrictions, max vessel size (not available elsewhere)
3. **Keep UPPLY enrichment** — LOCODEs, zone codes, vessel traffic counts
4. **Drop**: Ports that can't be geocoded to terminal level and have no traffic data
5. **Fix top 200 manually** — Cross-reference with satellite imagery for major ports

### Ports to Fix Immediately (Wrong Coordinates)
```
HAI PHONG:    20.92,106.68 → 20.82,106.78 (Lach Huyen Deep Water Port)
HO CHI MINH:  10.77,106.70 → 10.74,106.76 (Cat Lai Terminal)
SHANGHAI:     31.22,121.50 → 30.63,122.08 (Yangshan Deep Water Port)
GUANGZHOU:    23.07,113.37 → 22.60,113.60 (Nansha Port)
NINGBO:       29.87,121.55 → 29.93,121.85 (Beilun Container Terminal)
TIANJIN:      39.17,117.18 → 38.98,117.73 (Tianjin Port Container Terminal)
MUMBAI:       18.95,72.85 → 18.95,72.95 (JNPT/Nhava Sheva)
```

---

## Domain: Maritime (Sea Distances)

### Current State
- 23,293 port pairs from NGA PUB 151 (via kaklin/sea-routes parser)
- Port names use 1990s conventions ("Haiphong, Vietnam" not "Hai Phong")
- No routing geometry (straight lines only)

### Recommended Sources

| Source | Records | Quality | Notes |
|--------|---------|---------|-------|
| **NGA PUB 151** (current) | 23k pairs | ★★★ | Only distance, no geometry. Names outdated. |
| **searoute** (npm/python) | On-demand | ★★★★★ | Actual sea routing via waypoints, avoids land. MIT license. |
| **marinetraffic.com** | N/A | ★★★★★ | Best but commercial |

### Action Plan

1. **Keep NGA PUB 151** as distance reference (authoritative for planning)
2. **Add**: Use `searoute` library to generate actual route geometry (GeoJSON polylines that follow sea lanes, not straight lines)
3. **Fix**: Name mapping table (NGA names → current port names)
4. **Drop**: Nothing — distance data is still valid even if names are old

---

## Domain: Maritime (Shipping Lanes)

### Current State
- 3 features (Major/Middle/Minor) from Paul Benden / CIA World Oceans Map
- Works now (filter fix deployed)
- **FLAW**: No coverage of Vietnam coast — Hai Phong and Cat Lai (Ho Chi Minh) are top 30 global ports but have zero lane connectivity in the data. South China Sea feeder routes to Vietnam are entirely missing.

### Gaps Identified
- Vietnam coast (Hai Phong, Cat Lai/HCMC, Cai Mep, Da Nang) — no lanes
- Several other high-volume feeder routes likely missing (e.g., intra-Asia routes)
- CIA source focuses on ocean trunk routes, ignores coastal/feeder lanes

### Action Plan
1. **Add missing lane segments** — Create supplementary GeoJSON with:
   - South China Sea → Hai Phong feeder (Singapore/HK → Hai Phong via Hainan Strait)
   - South China Sea → Cat Lai/Cai Mep feeder (Malacca → Vung Tau → Cat Lai)
   - South China Sea → Da Nang
   - Other missing high-traffic feeders (India west coast, East Africa, etc.)
2. **Source**: Manual trace from MarineTraffic density maps + AIS track patterns
3. **Classification**: Vietnam feeders = "Middle" tier (high-volume but not trunk routes)
4. **Format**: Append new LineString features to existing GeoJSON with same property schema
5. **Alternative**: Consider replacing entirely with a more complete dataset (e.g., generate from AIS density data via Global Fishing Watch)

### Assessment
- Base dataset is OK for ocean trunk routes
- Needs supplementary feeder lanes for top 50 ports not directly on trunk routes

---

## Domain: Maritime (Ships)

### Current State
- 746k ships from ITU List V 2025
- Call sign, name, MMSI, country, tonnage, type, dimensions

### Assessment
- ✅ **Keep as-is** — ITU List V is the authoritative source, 2025 edition is current
- Could supplement with MarineTraffic's free vessel photos/details API for top vessels

---

## Domain: Aviation (Aircraft)

### Current State
- 566k aircraft from adsbdb.com
- Mode-S hex, registration, type, manufacturer, model, owner

### Recommended Sources

| Source | Records | Quality | Notes |
|--------|---------|---------|-------|
| **adsbdb.com** (current) | 566k | ★★★★ | Community-sourced, mostly current |
| **OpenSky Network** | ~600k | ★★★★★ | Academic, comprehensive, API available |
| **adsb.lol** | 500k+ | ★★★★ | Open, real-time data lake |
| **FAA Registry** | 300k+ | ★★★★★ | Authoritative for N-numbers |

### Action Plan

1. ✅ **Keep adsbdb** as primary (good enough, 566k records)
2. **Supplement**: Cross-reference FAA registry for N-numbers (correct owner data)
3. **Update tool**: Already exists (`cmd/update/`) — run periodically against adsbdb API
4. **Consider**: OpenSky metadata dump for non-US registrations

---

## Domain: Aviation (Flight Routes)

### Current State
- 507k callsign→route mappings from adsbdb.com
- Format: callsign, code, number, airline_code, airport_codes (e.g., "EGLL-KJFK")
- Many are historic/defunct routes

### Recommended Sources

| Source | Records | Quality | Notes |
|--------|---------|---------|-------|
| **adsbdb.com** (current) | 507k | ★★★ | Community, many stale |
| **Jonty/airline-route-data** | ~100k+ | ★★★★★ | Weekly auto-updated from OAG/FlightRadar. Current schedules. |
| **MrAirspace/aircraft-flight-schedules** | ~200k | ★★★★★ | ADS-B derived, quarterly since 2024 |

### Action Plan

1. **Replace stale routes** — Cross-reference with Jonty's weekly-updated dataset
2. **Add MrAirspace schedules** — Quarterly ADS-B derived, confirms active routes
3. **Drop**: Routes not seen in any 2024/2025 ADS-B data (probably defunct)
4. **Keep**: The update-tool for adding individual new routes on demand

### Quality Gate
- A route should appear in at least ONE of: Jonty (current schedules), MrAirspace (2024/2025 ADS-B), or adsbdb (recent lookup)
- Routes failing this = mark as "historic" or remove

---

## Domain: Aviation (Airlines)

### Current State
- 6,118 airlines from VRS Standing Data
- Many defunct airlines still present

### Recommended Sources

| Source | Records | Quality | Notes |
|--------|---------|---------|-------|
| **VRS Standing Data** (current) | 6k | ★★★ | Open but includes many defunct |
| **ICAO Doc 8585** | ~5k | ★★★★★ | Authoritative but not freely downloadable |
| **Jonty route data** (derived) | ~800 active | ★★★★★ | Only airlines with current routes |

### Action Plan

1. **Keep VRS** as base reference
2. **Flag active**: Cross-reference with Jonty route data — airlines not in any current route → mark defunct
3. **Drop**: Airlines with no ICAO code and no routes (noise)

---

## Domain: Aviation (Airports)

### Current State
- 8,001 airports from OurAirports
- Includes ICAO, IATA, coordinates, elevation

### Assessment
- ✅ **Keep as-is** — OurAirports is actively maintained, community-updated
- Coordinates are runway-level accurate (not city center)
- Could filter to only airports with scheduled service (~3,000) for the map layer

---

## Implementation Priority

### Phase 1 — Immediate Fixes (v1.0.3)
- [x] Fix top 29 port coordinates (terminal-level)
- [x] Shipping lanes filter fix (`Type` not `type`)
- [ ] Add missing shipping lane segments (Vietnam coast feeders: HP, Cat Lai, Cai Mep)
- [ ] Remove ports with clearly wrong data (negative port_size values, etc.)
- [ ] Validate top 50 port coordinates against satellite imagery

### Phase 2 — Port Overhaul (v1.1.0)
- [ ] Download tayljordan/ports dataset
- [ ] Build merge script: keep NGA metadata + UPPLY enrichment, replace coordinates
- [ ] Validate: all Major/Large ports should have terminal-level coordinates
- [ ] Add searoute geometry generation for top 100 port pairs

### Phase 3 — Route Freshness (v1.2.0)
- [ ] Download Jonty/airline-route-data (weekly JSON)
- [ ] Download MrAirspace Q1 2025 schedule data
- [ ] Build reconciliation script: mark stale routes, add new ones
- [ ] Add "last_seen" or "confidence" field to routes

### Phase 4 — Full Pipeline (v2.0.0)
- [ ] Automated weekly pull from Jonty route data
- [ ] Quarterly pull from MrAirspace ADS-B schedules
- [ ] Automated port coord validation (satellite imagery cross-ref)
- [ ] Remove all data not confirmed active in 2024/2025

---

## Data to DROP

| Dataset | Reason |
|---------|--------|
| Routes not seen since 2023 | Stale/defunct |
| Airlines with no ICAO and no routes | Noise |
| Ports with no LOCODE, no traffic, no depth data | Unverifiable |
| Aircraft with invalid Mode-S hex | Data corruption |
| Sea distance entries for ports that no longer exist | Confusing |

---

## Data Sources URLs

```
# Ports (coordinates)
https://github.com/tayljordan/ports                    # 3,898 ports, terminal-level coords
https://opendata.upply.com/seaports                    # LOCODEs, zones
https://msi.nga.mil/Publications/WPI                   # NGA WPI Pub 150

# Routes (aviation)
https://github.com/Jonty/airline-route-data            # Weekly updated airline routes
https://github.com/MrAirspace/aircraft-flight-schedules # Quarterly ADS-B schedules

# Shipping
https://github.com/newzealandpaul/Shipping-Lanes       # Shipping lanes GeoJSON
https://github.com/kaklin/sea-routes                   # NGA PUB 151 parser
https://pypi.org/project/searoute/                     # Sea routing geometry

# Aircraft
https://www.adsbdb.com/                                # Aircraft + route lookups
https://github.com/adsblol/globe_history_2025          # ADS-B raw data

# Airports
https://ourairports.com/data/                          # Best airport data
```
