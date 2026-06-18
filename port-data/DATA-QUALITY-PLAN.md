# Data Quality Plan: Seaport Modernization

## Problem
The NGA World Port Index (WPI) backbone data is ~20 years outdated:
- Hai Phong → "Small" (actually top 13 worldwide, 7M+ TEU)
- Ningbo → "Small" (actually world #1 container port, 35M+ TEU)
- Guangzhou → "Minor" (actually #5, 25M+ TEU)
- Tanjung Pelepas → "Minor" (actually top 20, 11M+ TEU)
- Colombo → "Minor" (actually major hub, 7M+ TEU)
- Laem Chabang → "Small" (actually top 20, 10M+ TEU)
- Many key ports missing LOCODE

The port_size field in WPI reflects early-2000s harbor infrastructure, not actual traffic/volume.

## Strategy: Quality over Quantity

### Phase 1: Reclassify by Real Traffic Data
Source: **Lloyd's List Top 100 Container Ports** (published annually) + **UNCTAD Review of Maritime Transport**

New classification criteria:
| Tier | TEU Volume | Count |
|------|-----------|-------|
| Major | >5M TEU/year OR top 50 worldwide | ~50 |
| Large | 1M–5M TEU OR significant regional hub | ~100 |
| Medium | 100k–1M TEU | ~300 |
| Small | <100k TEU but active | rest |

### Phase 2: Source Modern Data
1. **UNCTAD port stats** (public): TEU data for 800+ ports
2. **Lloyd's List Top 100** (2024): definitive container ranking
3. **World Shipping Council** membership ports
4. **IAPH (International Association of Ports)** member directory
5. **Wikipedia container port lists** (well-sourced, cross-referenced)

### Phase 3: Enrich Missing Fields
- Add LOCODE to the 1,682 ports without one (use UN/LOCODE database)
- Update coordinates for ports that have moved (new terminals built on reclaimed land)
- Add modern capacity/TEU data as new field

### Implementation

#### Script: `port-data/modernize.py`
1. Load current seaports.csv
2. Load Lloyd's Top 100 + UNCTAD data → override port_size
3. Load UN/LOCODE → fill missing LOCODEs
4. Manual corrections for known errors (Hai Phong, Ningbo, etc.)
5. Output updated seaports.csv

#### Immediate Manual Fixes (can do now)
These are indisputable — world's top 30 container ports wrongly classified:

```
HAI PHONG → Major (VNHPH)
NINGBO → Major (CNNBO) — world #1
GUANGZHOU → Major (CNGGZ) — world #5
LAEM CHABANG → Major (THLCH) — top 20
TANJUNG PELEPAS → Major (MYTPP) — top 20
COLOMBO → Major (LKCMB) — top 25
PIRAEUS → Large (GRPIR) — top 30
```

#### Data Sources for Automation
- UN/LOCODE: https://unece.org/trade/cefact/unlocode-code-list-country-and-territory
- UNCTAD: https://unctadstat.unctad.org/datacentre/dataviewer/US.ContainerPortThroughput
- World Bank port data: datasets on container throughput
- Wikipedia: "List of busiest container ports" (well-sourced, auditable)

## Timeline
1. Immediate: manual fix top 30 misclassified ports (~30 min)
2. Short-term: script to merge UNCTAD TEU data for 800+ ports (1-2h)
3. Medium-term: full UN/LOCODE gap-fill (1h)
4. Tag as v1.0.1 (data quality patch)
