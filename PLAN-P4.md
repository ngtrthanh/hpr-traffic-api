# Phase 4 Detailed Plan: HPR Traffic Demo App

## Core Principle
Fork hpr-marine's shell as blueprint. Same design system, same layout, same interaction patterns. Replace AIS streaming with layered REST data exploration.

---

## LAYERS (all toggleable from left rail)

### Maritime Layers
| # | Layer | Source | Map Type | Icon/Visual | Count |
|---|-------|--------|----------|-------------|-------|
| 1 | **Seaports** | `/v1/ports/geojson` | circle | Color by size (Major=orange, Large=yellow, Medium=cyan, Small=slate) | 3,630 |
| 2 | **Shipping Lanes** | `/v1/shipping-lanes` | line | Red/amber/gray by tier (Major/Middle/Minor) | 3 tiers |
| 3 | **Sea Routes** | `/v1/sea-routes/from/{port}` | line (dynamic) | Accent-colored arcs drawn on port selection | on-demand |

### Aviation Layers
| # | Layer | Source | Map Type | Icon/Visual | Count |
|---|-------|--------|----------|-------------|-------|
| 4 | **Airports** | `/v1/airports/geojson` (new endpoint) | circle | Blue dots, sized by route count from top_airports | 3,862 |
| 5 | **Route Arcs** | `/v1/routes/{callsign}` + airport coords | line (dynamic) | Purple arcs on airport/route selection | on-demand |

### Layer Controls (left rail → Layers popover)
```
☑ Seaports        ●(orange)  3,630
☑ Shipping Lanes  —(red)     3 tiers
☐ Airports        ●(blue)    3,862
☐ Sea Routes      —(cyan)    on select
☐ Flight Routes   —(purple)  on select
```
Maritime layers ON by default. Aviation OFF (user toggles on). This avoids visual overload.

---

## LEFT RAIL (vertical nav, same pattern as hpr-marine)

```
┌──────────┐
│ ⊞ Layers │  → popover: toggle layers on/off with counts
│ 🗺 Map    │  → popover: style selector (Dark, Light, Satellite, Ocean, Positron)
│ 📍Region │  → popover: Global, Europe, Asia, Americas, Middle East, Oceania
│ 📋 List   │  → open left panel with data tables
│ 🔌 API    │  → open left panel with API guide
│           │
│ ─ spacer ─│
│ ⚙ Settings│  → popover: links (GitHub, MCP, docs)
└──────────┘
```

---

## LEFT PANEL (`#vlist` area, 280px)

### When "List" is active — Data Explorer
**Tabs**: Ports | Airports | Airlines | Ships

#### Ports Tab
- Scrollable rows: `[●] PORT NAME — Country (LOCODE)`
- Click row → fly to port on map + open right card
- Filter by typing in topbar search
- Sort: alphabetical (default), by size

#### Airports Tab
- Scrollable rows: `[●] ICAO — Name, Country`
- Click row → fly to airport + show airport card with route count + top routes

#### Airlines Tab  
- Scrollable rows: `ICAO — Name (route count)`
- Click row → show airline card with top routes listed

#### Ships Tab (search-only, 746k too many to list)
- Prompt: "Enter MMSI or callsign to search"
- Results appear as rows after search

### When "API" is active — Mini Guide Panel
**Tabs**: Quick Start | Endpoints | MCP | Code

#### Quick Start Tab
```
HPR Traffic API
Base URL: https://traffic.hpradar.com

Try it:
  curl /v1/routes/BAW123
  curl /v1/ports/NLRTM
  curl /v1/ships/353800000
```

#### Endpoints Tab
Grouped accordion:
- **Aviation** — `/v1/routes/`, `/v1/airlines/`, `/v1/airports/`, `/v0/aircraft/`
- **Maritime** — `/v1/ports/`, `/v1/ships/`, `/v1/sea-routes/`, `/v1/shipping-lanes`
- **Batch** — `POST /v1/batch/routes`, `POST /v1/batch/ships`
- **Meta** — `/v1/stats`, `/v1/mcp`

Each endpoint shows: method, path, brief description, example response (truncated).

#### MCP Tab
```
Connect with Claude/Cursor:
  URL: https://traffic.hpradar.com/v1/mcp

Available tools:
  • lookup_flight_route
  • lookup_aircraft
  • lookup_ship
  • lookup_port
  • sea_distance
  • nearby_ports
  • search_sea_routes
```

#### Code Tab
Code snippets (tabbed: curl | Python | JavaScript | Go):
```javascript
// Lookup a flight route
const res = await fetch('https://traffic.hpradar.com/v1/routes/BAW123');
const route = await res.json();
console.log(route.airport_codes); // "EGLL-OTHH"
```

---

## RIGHT CARD (`#pcard`, 320px) — Detail Card

### Port Card (on port click/list select)
```
┌─────────────────────────────┐
│ 🇳🇱 ROTTERDAM          Major │  ← flag + name + size badge
│ Netherlands · NLRTM · EU-NEU│  ← meta line
├─────────────────────────────┤
│ Max Vessel      large vessels│
│ Channel Depth         34.7 m│
│ Cargo Depth           45.7 m│
│ Tidal Range            1.0 m│
│ Entrance        other       │
│ WPI ID             31140    │
├─────────────────────────────┤
│ Sea Routes (12 destinations)│  ← section header
│  → Antwerp           52 nm  │  ← clickable, draws arc
│  → London           312 nm  │
│  → Hamburg          420 nm  │
│  → ...more                  │
├─────────────────────────────┤
│ [Try API] [View JSON]       │  ← action buttons
└─────────────────────────────┘
```

### Airport Card (on airport click/list select)
```
┌─────────────────────────────┐
│ ✈ EGLL — Heathrow     6,950r│  ← ICAO, name, route count
│ London, United Kingdom      │
├─────────────────────────────┤
│ IATA             LHR        │
│ Elevation        83 ft      │
│ Lat/Lon    51.47 / -0.46    │
├─────────────────────────────┤
│ Top Routes from EGLL        │  ← section
│  ✈ BAW123  → KJFK           │  ← clickable, draws arc
│  ✈ BAW456  → LEMD           │
│  ✈ EZY789  → LFPG           │
├─────────────────────────────┤
│ [Try API] [View JSON]       │
└─────────────────────────────┘
```

### Ship Card (from search)
```
┌─────────────────────────────┐
│ 🚢 EVER BREED         Cargo │
│ Panama · MMSI 353800000     │
├─────────────────────────────┤
│ Call Sign          H3BV     │
│ Gross Tonnage     32,691    │
│ Length               211 m  │
│ Beam                  33 m  │
│ Ship Type              74   │
├─────────────────────────────┤
│ [Try API] [View JSON]       │
└─────────────────────────────┘
```

### Route Card (from search/airport drill-down)
```
┌─────────────────────────────┐
│ ✈ BAW123           British A│
│ EGLL → KJFK                 │  ← draws arc on map
├─────────────────────────────┤
│ Airline       BAW           │
│ Number        123           │
│ Origin        EGLL (LHR)    │
│ Destination   KJFK (JFK)    │
├─────────────────────────────┤
│ [Try API] [View JSON]       │
└─────────────────────────────┘
```

---

## HOVER TOOLTIP (map hover on any icon)

Same pattern as hpr-marine vessel hover — lightweight popup (no card open):
- **Port hover**: `PORT NAME (Size) — Country`
- **Airport hover**: `ICAO — Name`
- Disappears on mouse leave. Full card opens on click.

---

## TOPBAR

```
┌─────────────────────────────────────────────────────────────────────┐
│ ⚓ HPRadar Traffic │ [Search ports, ships, flights...] │ 507k routes │ 3.6k ports │ 747k ships │ [🌙] │
└─────────────────────────────────────────────────────────────────────┘
```

- Brand: "HPRadar Traffic" with anchor icon
- Search: unified (same debounced search pattern from hpr-marine)
- Stats pills: route count, port count, ship count (from `/v1/stats`)
- Theme toggle (same sun/moon icon swap)

---

## SEARCH (topbar input)

Behavior (debounced 300ms):
1. If input matches `^\d{9}$` → ship MMSI lookup → `/v1/ships/{mmsi}`
2. If input matches `^[A-Z]{2,4}\d+` → flight callsign → `/v1/routes/{callsign}`
3. If input matches `^[A-Z]{4}$` → airport ICAO → fly to airport
4. If input matches `^[A-Z]{2}[A-Z]{3}$` → port LOCODE → `/v1/ports/{locode}`
5. Otherwise → port name search → `/v1/sea-routes/search?q=`

Results dropdown (same `#searchResults` pattern from hpr-marine):
```
🚢 EVER BREED (353800000) — Panama
✈️ BAW123: EGLL → KJFK
⚓ ROTTERDAM (NLRTM) — Netherlands
⚓ Singapore
```
Click result → fly to location (if has coords) + open card.

---

## NEW ENDPOINTS NEEDED (add to main.go)

### `GET /v1/airports/geojson`
Returns all airports as GeoJSON FeatureCollection for the airport map layer.
```json
{"type":"FeatureCollection","features":[
  {"type":"Feature","geometry":{"type":"Point","coordinates":[-0.461,51.47]},
   "properties":{"icao":"EGLL","iata":"LHR","name":"Heathrow","country":"United Kingdom","route_count":6950}}
]}
```
Pre-compute `route_count` per airport at startup (count routes containing that ICAO).

### `GET /v1/airports/{icao}`
Return airport details + top routes (already implied by `/v1/airports/` but currently returns routes, not airport info). May need a new dedicated endpoint or reuse existing.

---

## IMPLEMENTATION ORDER

1. **Backend additions** (~10 min)
   - Add `/v1/airports/geojson` endpoint
   - Pre-compute airport route counts at startup

2. **Vendor files** (~2 min)
   - Copy from hpr-marine: `js/maplibre-gl.js`, `css/maplibre-gl.css`, `mapstyles/`

3. **Fork style.css** (~15 min)
   - Copy hpr-marine `css/style.css`
   - Remove: decoder styles, vessel-specific styles, weather, feeder
   - Add: port/airport color vars, API guide panel styles, code block styles

4. **Build index.html** (~15 min)
   - Same shell structure: `#map`, `#shell`, `#topbar`, `#rail`, `#vlist`, `#pcard`
   - Adapted for traffic data (no decoder view, no feeder)
   - Add API guide panel HTML structure

5. **Build app.js** (~40 min)
   - Map init (same pattern: `MAP_STYLES`, `getMapStyle()`, globe projection)
   - Layer loading: fetch GeoJSON, add sources+layers
   - Layer toggles
   - Port/airport click → card rendering
   - Sea route arc drawing
   - Search logic (unified, debounced)
   - List panel (tabs: Ports/Airports/Airlines)
   - API guide panel (static content, code highlight)
   - Theme toggle + persist
   - Region presets
   - Coordinate display

6. **Update Dockerfile + embed** (~2 min)

7. **Deploy + test** (~5 min)

8. **Commit, tag v1.1.0, push**

---

## FILE SIZE BUDGET
- `index.html`: ~250 lines (shell only, no inline JS)
- `css/style.css`: ~800 lines (forked, trimmed)
- `js/app.js`: ~1200 lines (all logic)
- `mapstyles/`: 2 files, ~6KB each
- Total static: ~2.3 MB (mostly maplibre-gl.js at 1MB)
