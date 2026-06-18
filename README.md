# HPR Traffic API

A high-performance REST API serving aviation flight routes and maritime port/sea-route data. Zero external dependencies, pure Go stdlib, in-memory datasets — 18k+ requests/second on commodity hardware.

**Live at** [traffic.hpradar.com](https://traffic.hpradar.com) · **Marine demo** at [marine.hpradar.com](https://marine.hpradar.com)

![HPRadar Marine](https://raw.githubusercontent.com/ngtrthanh/hpr-marine/main/marine.png)

## Quick Start

```bash
docker compose up -d --build
```

The API will be available at `http://localhost:8081`.

## What's Inside

| Dataset | Records | Source |
|---------|---------|--------|
| Aircraft | 566k | [adsbdb.com](https://www.adsbdb.com/) |
| Flight Routes | 507k | [adsbdb.com](https://www.adsbdb.com/) |
| Airlines | 6,118 | [VRS Standing Data](https://github.com/vradarserver/standing-data) |
| Airports | 8,001 | [OurAirports](https://ourairports.com/) |
| Ships | 746k | [ITU List V 2025](https://www.itu.int/en/publications/Pages/default.aspx) |
| Seaports | 3,630 | [NGA World Port Index](https://msi.nga.mil/Publications/WPI) |
| Sea Distances | 23,293 pairs | [NGA PUB 151](https://msi.nga.mil/Publications/Distances) |
| Shipping Lanes | 3 tiers | [CIA World Oceans Map](https://github.com/newzealandpaul/Shipping-Lanes) |

---

## Aviation Endpoints

### Callsign Route Lookup

```
GET /v1/routes/{callsign}
```

```bash
curl http://localhost:8081/v1/routes/BAW123
```

```json
{
  "callsign": "BAW123",
  "code": "BAW",
  "number": "123",
  "airline_code": "BAW",
  "airport_codes": "EGLL-KJFK"
}
```

### Routes by Airline

```
GET /v1/airlines/{icao_code}?limit=50&offset=0
```

Returns all routes operated by an airline. Paginated (max 200 per page).

### Routes by Airport

```
GET /v1/airports/{icao_code}?limit=50&offset=0
```

Returns all routes through an airport with connected-airport ranking.

### Database Stats

```
GET /v1/stats
```

### adsbdb-Compatible (v0)

Drop-in replacement for [adsbdb.com](https://www.adsbdb.com/) API:

```
GET /v0/aircraft/{mode_s_or_registration}
GET /v0/callsign/{callsign}
GET /v0/airline/{icao}
GET /v0/n-number/{n_number}
GET /v0/mode-s/{hex}
```

---

## Maritime Endpoints

### Port Lookup

```
GET /v1/ports/{locode_or_wpi_id}
```

```bash
curl http://localhost:8081/v1/ports/NLRTM
```

```json
{
  "wpi_id": "31140",
  "name": "ROTTERDAM",
  "country": "Netherlands",
  "state": "South Holland",
  "latitude": 51.9,
  "longitude": 4.48,
  "port_size": "Major",
  "max_vessel_size": "large vessels",
  "channel_depth_m": 34.7,
  "cargo_depth_m": 45.7,
  "tidal_range_m": 1,
  "entrance_restriction": "other",
  "locode": "NLRTM",
  "zone_code": "EU-NEU"
}
```

### Ports by Country

```
GET /v1/ports/country/{country_name}?limit=50&offset=0
```

### Ports by Zone

```
GET /v1/ports/zone/{zone_code}?limit=50&offset=0
```

### Nearby Ports

```
GET /v1/ports/nearby?lat=1.3&lon=103.8&radius_km=50&limit=20
```

Returns ports within radius, sorted by distance.

### Port Stats

```
GET /v1/ports/stats
```

### Ship Lookup

```
GET /v1/ships/{mmsi}
```

```bash
curl http://localhost:8081/v1/ships/353800000
```

```json
{
  "mmsi": "353800000",
  "call_sign": "H3BV",
  "name": "EVER BREED",
  "country": "PNR",
  "gross_tonnage": 32691,
  "ship_type": 74,
  "length_m": 211,
  "beam_m": 33,
  "class": "MM"
}
```

### Ship by Callsign

```
GET /v1/ships/callsign/{callsign}
```

### Sea Distances (NGA PUB 151)

```
GET /v1/sea-routes/from/{port_name}
```

```bash
curl http://localhost:8081/v1/sea-routes/from/Singapore
```

```json
{
  "origin": "Singapore",
  "destinations": [
    {"origin": "Singapore", "destination": "Tandjung Uban, Indonesia", "distance_nm": 27, "type": "port"},
    {"origin": "Singapore", "destination": "Malacca, Malaysia", "distance_nm": 117, "type": "port"}
  ],
  "junctions": [
    {"origin": "Singapore", "destination": "Selat Sunda, Indonesia", "distance_nm": 532, "type": "junction"}
  ]
}
```

### Search Sea Route Ports

```
GET /v1/sea-routes/search?q=rotterdam
```

### Shipping Lanes (GeoJSON)

```
GET /v1/shipping-lanes
```

Returns full GeoJSON FeatureCollection with Major/Middle/Minor global shipping lane polylines. Suitable for direct rendering on MapLibre/Leaflet/Mapbox.

---

## Performance

Benchmarked with Apache Bench on 16-core host:

| Concurrency | Requests/sec | Median Latency | p99 Latency |
|-------------|-------------|----------------|-------------|
| 10 | 18,312 | <1ms | 2ms |
| 50 | 16,278 | 3ms | 4ms |
| 100 | 16,449 | 6ms | 10ms |

Startup: loads all datasets in ~3 seconds. Memory: ~1.5 GB resident.

## Update Tool

CLI to add new flight routes by querying the adsbdb API:

```bash
go build -o update-tool ./cmd/update/
./update-tool -callsigns "RYR99ZZ,BAW456"
./update-tool -input new_callsigns.txt -delay 200ms
```

## Docker

```bash
docker compose up -d --build    # Start
docker compose logs -f          # Logs
docker compose down             # Stop
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│  main.go (single file, zero dependencies)       │
│                                                 │
│  Middleware: Logging → CORS → Rate Limit (600/min/IP)
│                                                 │
│  Data:  CSV files → in-memory maps → O(1) lookup│
│         GeoJSON → raw bytes → direct serve      │
│                                                 │
│  Build: Go 1.24, multi-stage Docker             │
└─────────────────────────────────────────────────┘
```

## Data Credits & Licenses

This project integrates multiple open datasets. Full credit to their creators:

| Dataset | Provider | License/Notes |
|---------|----------|---------------|
| Flight routes & aircraft | [adsbdb.com](https://www.adsbdb.com/) — David Taylor (Edinburgh) & Jim Mason (Glasgow) | Community-sourced ADS-B data |
| Airline reference | [VRS Standing Data](https://github.com/vradarserver/standing-data) | Open source |
| Airport reference | [OurAirports](https://ourairports.com/data/) | Public domain |
| Seaport data (WPI) | [NGA World Port Index](https://msi.nga.mil/Publications/WPI) — enriched by [Jordan Taylor](https://linkedin.com/in/tayljordan) | US Government public domain |
| Ship registry | [ITU List V 2025](https://www.itu.int/en/publications/Pages/default.aspx) — International Telecommunication Union | ITU publication |
| Sea distances (PUB 151) | [NGA Distances Between Ports](https://msi.nga.mil/Publications/Distances) — parsed by [kaklin](https://github.com/kaklin/sea-routes) | US Government public domain |
| Shipping lanes | [Paul Benden](https://github.com/newzealandpaul/Shipping-Lanes) / CIA World Oceans Map | CC BY 4.0 |
| Port LOCODE reference | [UPPLY](https://www.upply.com/) | Reference data |
| Vessel traffic stats | [MarineTraffic](https://www.marinetraffic.com/) / community data | Aggregated statistics |

## License

MIT

---

Built by [HPRadar](https://hpradar.com) · Aviation + Maritime intelligence for the open web.
