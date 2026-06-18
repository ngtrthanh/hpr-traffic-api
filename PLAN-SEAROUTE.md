# Plan: Sea Route Geometry (Realistic Shipping Lanes)

## Problem
- Current straight-line feeder lanes look silly (deleted)
- Need realistic maritime routes that follow actual sea lanes, avoid land
- Routes should be precomputed GeoJSON polylines, not on-demand computation

## Research Summary

### eurostat/searoute (Java, EUPL license)
- **Algorithm**: Dijkstra's shortest path on a maritime network graph
- **Network**: Oak Ridge National Labs CTA Transportation Network (2000), enriched with AIS data
- **Data format**: GeoPackage (.gpkg) at 5km/10km/20km/50km/100km resolutions
- **Supports**: Suez, Panama, Malacca, Gibraltar, Dover, Bering, Magellan, Bab-el-Mandeb, Kiel, Corinth straits
- **Output**: GeoJSON LineStrings with distance
- **Key insight**: The NETWORK DATA (marnet_plus_*.gpkg) is the real asset — it's a graph of maritime edges covering global seas

### dakk/gweatherrouting (Python, GPL-3.0)
- **Purpose**: Sailing weather routing (wind-optimized paths)
- **Not suitable**: Different problem domain (sailing optimization, not commercial shipping)
- **Useful**: Has GeoJSON coastline data for land avoidance

### searoute (Python, pypi.org/project/searoute)
- Python port, uses same marnet concept
- "Developed to generate realistic-looking sea routes for visualizations"
- Exactly what we need, but in Python

## Implementation Plan: Go Sea Router

### Approach: Port eurostat's marnet concept to Go
Since we require Go (zero external deps, single binary), implement a lightweight sea routing engine:

1. **Extract the maritime network** from eurostat's `.gpkg` files into GeoJSON
2. **Build a graph** in Go: nodes (waypoints) + edges (maritime links with distances)
3. **Implement Dijkstra** for shortest maritime path between any two coordinates
4. **Pre-compute top routes**: For our 1,247 sea route origins × their destinations, generate polyline geometries
5. **Serve as GeoJSON**: Replace straight-line arcs with real sea-following polylines

### Data Pipeline

```
eurostat/searoute marnet_plus_20km.gpkg
    ↓ (one-time extraction via ogr2ogr or Python)
marnet_20km.geojson (maritime network as LineStrings)
    ↓ (Go at startup)
Graph: map[nodeID][]Edge
    ↓ (Dijkstra at request time OR pre-computed)
GeoJSON polyline: port A → port B following sea lanes
```

### Architecture Options

| Option | Pros | Cons |
|--------|------|------|
| **A: Pre-compute all** | Fast serve (O(1)), no runtime overhead | Large file (~50MB for 23k routes), one-time build |
| **B: On-demand Dijkstra** | Small data footprint (~2MB network), fresh routes | ~5-20ms per route computation |
| **C: Hybrid** | Pre-compute top 500 pairs, on-demand for rest | Best of both, moderate complexity |

**Recommendation: Option B** — Load the 20km network (~8k nodes, ~12k edges) into memory at startup. Run Dijkstra on demand. At 20km resolution, the graph is small enough for <10ms routing.

### Implementation Steps

1. **Extract network**: Convert `marnet_plus_20km.gpkg` → `marnet.geojson` (one-time, using ogr2ogr)
2. **Go graph loader**: Parse GeoJSON LineStrings → build adjacency list with haversine weights
3. **Dijkstra router**: Given (lat1,lon1) → (lat2,lon2):
   - Find nearest network nodes to origin/destination (snap to graph)
   - Run Dijkstra
   - Return path as coordinate array
4. **New endpoint**: `GET /v1/sea-routes/path?from_lat=X&from_lon=Y&to_lat=X&to_lon=Y`
   - Returns GeoJSON Feature with LineString geometry
5. **Update geojson endpoint**: `/v1/sea-routes/geojson?from=PORT` now returns realistic polylines instead of straight lines
6. **Shipping lanes**: The marnet network itself IS the shipping lanes visualization (rendered as lines on the map, replaces the CIA dataset)

### File Structure
```
main.go          — add graph type, Dijkstra, sea route handler
marnet.geojson   — 20km maritime network (extracted from eurostat)
```

### Alternative: Use the CIA lanes + waypoint routing
If extracting from .gpkg is too complex, a simpler approach:
- Keep CIA shipping_lanes.geojson as the VISUAL layer
- For route geometry: route through a set of ~100 manually-defined waypoints (straits, capes, junctions)
- Less accurate but much simpler to implement

### Strait/Channel Handling
The network has `pass` property on edges for:
- Suez Canal
- Panama Canal
- Malacca Strait
- etc.

This allows route options (e.g., "avoid Suez" → route via Cape of Good Hope).

## Immediate Actions

1. Extract marnet_plus_20km.gpkg → GeoJSON (need ogr2ogr or Python with geopandas)
2. Implement Go graph + Dijkstra
3. Replace current shipping lanes with marnet network visualization
4. Replace straight-line sea route arcs with Dijkstra paths

## Dependencies
- `ogr2ogr` (GDAL) or Python `geopandas` — one-time data extraction only
- No runtime dependencies added to Go binary
