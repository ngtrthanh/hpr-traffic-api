# Social Media Posts — HPR Traffic API v1.0 Launch

---

## X (Twitter) — Thread

### Tweet 1 (Main)

🚀 Launching HPR Traffic API — a free, open REST API for aviation AND maritime traffic data.

• 507k flight routes
• 566k aircraft
• 746k ships (ITU registry + AIS dimensions)
• 3,630 seaports with depths & restrictions
• 23k sea distance pairs (NGA PUB 151)
• Global shipping lanes (GeoJSON)

Zero deps. Pure Go. 18k req/sec.

🔗 traffic.hpradar.com
📦 github.com/ngtrthanh/hpr-traffic-api

### Tweet 2

Aviation: drop-in replacement for adsbdb API + extended endpoints.

```
GET /v1/routes/BAW123 → EGLL-KJFK
GET /v1/airlines/RYR → all Ryanair routes
GET /v1/airports/EGLL → all Heathrow connections
```

Full callsign → airline → origin/destination resolution.

### Tweet 3

Maritime: 3,630 ports with channel depth, tidal data, entrance restrictions. Plus port-to-port sea distances from the official NGA PUB 151.

```
GET /v1/ports/NLRTM → Rotterdam (34.7m channel, Major)
GET /v1/sea-routes/from/Singapore → 150 destinations
GET /v1/ports/nearby?lat=1.3&lon=103.8 → nearest ports
```

### Tweet 4

All data is in-memory. Single Go binary. Loads in 3 seconds. Responds in microseconds.

Built on open datasets from:
🛫 @adabordb
🚢 NGA World Port Index
🗺️ CIA World Oceans Map
🏗️ OurAirports, VRS Standing Data

Credits & licenses in the README 👇

### Tweet 5

Live demo: marine.hpradar.com (real-time AIS tracking using this API's port & shipping lane data)

If you're building anything with flight tracking, port logistics, or maritime routing — this is your free backend.

Star ⭐ if useful: github.com/ngtrthanh/hpr-traffic-api

#opensource #aviation #maritime #golang #API

---

## Facebook Post

🚀 **Launching HPR Traffic API** — Free, open-source REST API for aviation and maritime data

After months of curating and cross-referencing multiple datasets, we're releasing a unified API that covers both skies and seas:

**Aviation:**
- 507,000 flight routes (callsign → airline → airports)
- 566,000 aircraft (Mode-S, registration, type, owner)
- 6,118 airlines, 8,001 airports
- Drop-in compatible with adsbdb.com API

**Maritime:**
- 3,630 seaports with operational data (depths, restrictions, vessel size limits)
- 23,293 port-to-port sea distances (from NGA PUB 151)
- Global shipping lanes as GeoJSON
- Nearby port search, zone/country filtering

**Technical highlights:**
- Single Go binary, zero external dependencies
- In-memory data store — responds in microseconds
- 18,000+ requests/second on standard hardware
- Multi-stage Docker build, ready to self-host

Built on open data from NGA (World Port Index, PUB 151), adsbdb.com, OurAirports, VRS Standing Data, and the CIA World Oceans Map. All properly credited.

🔗 Live: traffic.hpradar.com
🗺️ Marine demo: marine.hpradar.com
📦 Source: github.com/ngtrthanh/hpr-traffic-api

Free to use. MIT licensed. Star if it's useful to you!

#OpenSource #Aviation #Maritime #GoLang #API #FlightTracking #ShippingRoutes

---

## Blog Post (for hpradar.com/blog or Medium)

### Title: "We Built a Free API for 500k Flight Routes and 3,600 Seaports"

### Subtitle: One endpoint for aviation callsigns. One endpoint for port depths. Zero dependencies. 18k requests/second.

---

**The problem:** Aviation and maritime data is fragmented across dozens of sources — different formats, different update cycles, different APIs (if they even have one). If you're building a flight tracker or a maritime logistics tool, you spend more time wrangling data than writing features.

**The solution:** HPR Traffic API unifies the best open datasets into a single, fast REST API.

#### What's inside

| Domain | What you get |
|--------|-------------|
| Aviation | 507k callsign→route mappings, 566k aircraft by Mode-S/registration, 6k airlines, 8k airports |
| Maritime | 3,630 ports with depth/tidal/restriction data, 23k sea distances, global shipping lanes |

#### How it works

One Go file (`main.go`). At startup, it loads CSV files into in-memory maps. Every lookup is O(1). No database, no ORM, no framework.

The middleware stack is three functions: request logging, CORS, and a per-IP rate limiter (600/min sliding window). That's it.

#### Aviation endpoints

```bash
# What route does BAW123 fly?
curl traffic.hpradar.com/v1/routes/BAW123
# → {"callsign":"BAW123","airline_code":"BAW","airport_codes":"EGLL-KJFK"}

# All Ryanair routes (paginated)
curl traffic.hpradar.com/v1/airlines/RYR?limit=50

# All routes through Singapore Changi
curl traffic.hpradar.com/v1/airports/WSSS
```

It's also a drop-in replacement for the adsbdb.com API (`/v0/callsign/`, `/v0/aircraft/`, etc.), so existing integrations work without code changes.

#### Maritime endpoints

```bash
# Rotterdam port details
curl traffic.hpradar.com/v1/ports/NLRTM
# → depth: 34.7m, cargo: 45.7m, tidal: 1.0m, size: Major

# Sea distance from Singapore
curl traffic.hpradar.com/v1/sea-routes/from/Singapore
# → Tandjung Uban: 27nm, Malacca: 117nm, ... (150 destinations)

# Ports within 50km of a coordinate
curl "traffic.hpradar.com/v1/ports/nearby?lat=51.9&lon=4.5&radius_km=50"

# Global shipping lanes (GeoJSON, render directly on a map)
curl traffic.hpradar.com/v1/shipping-lanes
```

#### Performance

On a 16-core box with Apache Bench:

- 18,312 req/sec at concurrency 10
- Sub-millisecond median latency
- 10ms p99 at concurrency 100
- All datasets load in ~3 seconds at startup

Memory footprint: ~1.5 GB (566k aircraft + 507k routes + port data, all in RAM).

#### Data sources & credits

We didn't create this data — we curated, cleaned, and unified it. Full credit to:

- **Flight routes & aircraft**: [adsbdb.com](https://www.adsbdb.com/) — community-sourced ADS-B data by David Taylor and Jim Mason
- **Seaports**: [NGA World Port Index](https://msi.nga.mil/Publications/WPI) — enriched by Jordan Taylor
- **Sea distances**: [NGA PUB 151](https://msi.nga.mil/Publications/Distances) — parsed by [kaklin](https://github.com/kaklin/sea-routes)
- **Shipping lanes**: [Paul Benden](https://github.com/newzealandpaul/Shipping-Lanes) / CIA World Oceans Map (CC BY 4.0)
- **Airports**: [OurAirports](https://ourairports.com/) (public domain)
- **Airlines**: [VRS Standing Data](https://github.com/vradarserver/standing-data)

#### Self-host or use ours

```bash
# Run your own instance
docker compose up -d --build

# Or use the public API
curl traffic.hpradar.com/v1/routes/BAW123
```

Rate limited at 600 requests/min per IP on the public instance. No API key needed.

#### What's next

- AIS-based vessel route tracking (live shipping routes, not just distances)
- Historical route analytics
- Multi-hop sea distance calculation (A→B via junction points)

---

**Links:**
- API: [traffic.hpradar.com](https://traffic.hpradar.com)
- Marine demo: [marine.hpradar.com](https://marine.hpradar.com)
- Source: [github.com/ngtrthanh/hpr-traffic-api](https://github.com/ngtrthanh/hpr-traffic-api)
- License: MIT
