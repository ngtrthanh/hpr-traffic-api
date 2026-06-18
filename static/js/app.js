// ═══════════════════════════════════════════════════════════════
// HPR Traffic Demo — app.js
// Blueprint: hpr-marine (same patterns, adapted for REST API)
// ═══════════════════════════════════════════════════════════════

const API = '';

// ── Map Styles (same as hpr-marine) ──
const MAP_STYLES = {
  'Dark': 'mapstyles/dark-minimal.json',
  'Light': 'mapstyles/light-minimal.json',
  'Positron': 'https://tiles.openfreemap.org/styles/positron',
  'Liberty': 'https://tiles.openfreemap.org/styles/liberty',
  'ESRI Satellite': { url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}', attribution: '© Esri' },
  'ESRI Ocean': { url: 'https://server.arcgisonline.com/ArcGIS/rest/services/Ocean/World_Ocean_Base/MapServer/tile/{z}/{y}/{x}', attribution: '© Esri' },
  'CartoDB Dark': { url: 'https://basemaps.cartocdn.com/dark_all/{z}/{x}/{y}@2x.png', attribution: '© CartoDB © OSM' },
};
let curStyle = localStorage.getItem('trafficStyle') || 'Dark';

function getMapStyle(name) {
  const s = MAP_STYLES[name];
  if (typeof s === 'string') return s;
  return { version: 8, sources: { base: { type: 'raster', tiles: [s.url], tileSize: 256, attribution: s.attribution } }, layers: [{ id: 'base', type: 'raster', source: 'base' }] };
}

// ── Theme ──
function initTheme() {
  const t = localStorage.getItem('trafficTheme') || 'dark';
  document.documentElement.setAttribute('data-theme', t);
  setThemeIcon(t);
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme') || 'dark';
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('trafficTheme', next);
  setThemeIcon(next);
  const matchStyle = next === 'dark' ? 'Dark' : 'Light';
  if (curStyle !== matchStyle && MAP_STYLES[matchStyle]) switchStyle(matchStyle);
}
function setThemeIcon(t) {
  const btn = document.getElementById('theme-tog');
  btn.innerHTML = t === 'dark'
    ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>'
    : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>';
}
function switchStyle(name) {
  curStyle = name;
  localStorage.setItem('trafficStyle', name);
  map.setStyle(getMapStyle(name));
  map.once('style.load', () => { addAllLayers(); });
  buildMapMenu();
}
initTheme();

// ── Map Init ──
const map = new maplibregl.Map({
  container: 'map',
  style: getMapStyle(curStyle),
  center: [20, 20],
  zoom: 2,
  attributionControl: false,
});
map.addControl(new maplibregl.NavigationControl(), 'top-right');
map.addControl(new maplibregl.AttributionControl({ compact: true }), 'bottom-right');

// Coordinate display
const coordEl = document.createElement('div');
coordEl.id = 'coord-display';
document.getElementById('map').appendChild(coordEl);
map.on('mousemove', e => {
  const { lng, lat } = e.lngLat;
  const ns = lat >= 0 ? 'N' : 'S', ew = lng >= 0 ? 'E' : 'W';
  coordEl.textContent = `${Math.abs(lat).toFixed(4)}°${ns}  ${Math.abs(lng).toFixed(4)}°${ew}  Z${map.getZoom().toFixed(1)}`;
});

// ═══════════════════════════════════════════════════════════════
// LAYERS
// ═══════════════════════════════════════════════════════════════
const layers = { ports: true, lanes: true, airports: false };
let portsData = null, airportsData = null;

async function loadPorts() {
  // Load via binary protocol (10x smaller), convert to GeoJSON for MapLibre
  try {
    const buf = await fetch(API + '/v1/ports/bin').then(r => r.arrayBuffer());
    const dv = new DataView(buf);
    const count = dv.getUint16(6, true);
    const stOff = dv.getUint32(8, true);
    // Parse string table
    let off = stOff;
    const stCount = dv.getUint16(off, true); off += 2;
    const strings = [];
    for (let i = 0; i < stCount; i++) {
      const slen = dv.getUint16(off, true); off += 2;
      strings.push(new TextDecoder().decode(new Uint8Array(buf, off, slen))); off += slen;
    }
    const sizeNames = ['Very Small', 'Minor', 'Small', 'Medium', 'Large', 'Major'];
    const features = [];
    for (let i = 0; i < count; i++) {
      const o = 16 + i * 16;
      const lat = dv.getInt32(o, true) / 1e6;
      const lon = dv.getInt32(o + 4, true) / 1e6;
      const size = dv.getUint8(o + 8);
      const nameIdx = dv.getUint16(o + 9, true);
      const countryIdx = dv.getUint16(o + 11, true);
      const flags = dv.getUint8(o + 13);
      const teu = dv.getUint16(o + 14, true);
      const countryRaw = strings[countryIdx] || '';
      const [country, cc] = countryRaw.includes('|') ? countryRaw.split('|') : [countryRaw, ''];
      const flag = cc ? String.fromCodePoint(...[...cc].map(c => 0x1F1E6 + c.charCodeAt(0) - 65)) : '';
      features.push({ type: 'Feature', geometry: { type: 'Point', coordinates: [lon, lat] }, properties: {
        name: strings[nameIdx] || '', country, country_code: cc, flag,
        port_size: sizeNames[size] || 'Small', teu_thousands: teu, has_locode: !!(flags & 1)
      }});
    }
    portsData = { type: 'FeatureCollection', features };
  } catch(e) {
    // Fallback to JSON
    portsData = await fetch(API + '/v1/ports/geojson').then(r => r.json());
  }
  map.addSource('ports', { type: 'geojson', data: portsData });
  map.addLayer({ id: 'ports', type: 'circle', source: 'ports', paint: {
    'circle-radius': ['match', ['get', 'port_size'], 'Major', 6, 'Large', 4.5, 'Medium', 3.5, 2],
    'circle-color': ['match', ['get', 'port_size'], 'Major', '#f97316', 'Large', '#facc15', 'Medium', '#22d3ee', '#94a3b8'],
    'circle-stroke-width': 0.5, 'circle-stroke-color': '#fff', 'circle-opacity': 0.85
  }});
}

async function loadLanes() {
  const data = await fetch(API + '/v1/shipping-lanes').then(r => r.json());
  map.addSource('lanes', { type: 'geojson', data });
  map.addLayer({ id: 'lanes-major', type: 'line', source: 'lanes', filter: ['==', ['get', 'Type'], 'Major'], paint: { 'line-color': '#ef4444', 'line-width': 1.5, 'line-opacity': 0.6 } });
  map.addLayer({ id: 'lanes-middle', type: 'line', source: 'lanes', filter: ['==', ['get', 'Type'], 'Middle'], paint: { 'line-color': '#f59e0b', 'line-width': 1, 'line-opacity': 0.4 } });
  map.addLayer({ id: 'lanes-minor', type: 'line', source: 'lanes', filter: ['==', ['get', 'Type'], 'Minor'], paint: { 'line-color': '#6b7280', 'line-width': 0.5, 'line-opacity': 0.3 } });
}

async function loadAirports() {
  if (airportsData) { setLayerVis('airports', true); return; }
  try {
    const buf = await fetch(API + '/v1/airports/bin').then(r => r.arrayBuffer());
    const dv = new DataView(buf);
    const count = dv.getUint16(6, true);
    const stOff = dv.getUint32(8, true);
    let off = stOff;
    const stCount = dv.getUint16(off, true); off += 2;
    const strings = [];
    for (let i = 0; i < stCount; i++) {
      const slen = dv.getUint16(off, true); off += 2;
      strings.push(new TextDecoder().decode(new Uint8Array(buf, off, slen))); off += slen;
    }
    const features = [];
    for (let i = 0; i < count; i++) {
      const o = 16 + i * 16;
      const lat = dv.getInt32(o, true) / 1e6;
      const lon = dv.getInt32(o + 4, true) / 1e6;
      const nameIdx = dv.getUint16(o + 8, true);
      const icaoIdx = dv.getUint16(o + 10, true);
      const iataIdx = dv.getUint16(o + 12, true);
      const routeCount = dv.getUint16(o + 14, true);
      features.push({ type: 'Feature', geometry: { type: 'Point', coordinates: [lon, lat] }, properties: {
        name: strings[nameIdx] || '', icao: strings[icaoIdx] || '', iata: strings[iataIdx] || '', route_count: routeCount
      }});
    }
    airportsData = { type: 'FeatureCollection', features };
  } catch(e) {
    airportsData = await fetch(API + '/v1/airports/geojson').then(r => r.json());
  }
  map.addSource('airports', { type: 'geojson', data: airportsData });
  map.addLayer({ id: 'airports', type: 'circle', source: 'airports', paint: {
    'circle-radius': ['interpolate', ['linear'], ['get', 'route_count'], 0, 2, 1000, 4, 10000, 7],
    'circle-color': '#60a5fa', 'circle-stroke-width': 0.5, 'circle-stroke-color': '#fff', 'circle-opacity': 0.8
  }});
  setupAirportInteractions();
}

function setLayerVis(id, vis) {
  const v = vis ? 'visible' : 'none';
  if (id === 'lanes') { ['lanes-major', 'lanes-middle', 'lanes-minor'].forEach(l => { if (map.getLayer(l)) map.setLayoutProperty(l, 'visibility', v); }); }
  else { if (map.getLayer(id)) map.setLayoutProperty(id, 'visibility', v); }
}

function toggleLayer(name) {
  layers[name] = !layers[name];
  if (name === 'airports' && layers[name]) { loadAirports(); }
  else if (name === 'airports') { setLayerVis('airports', false); }
  else { setLayerVis(name, layers[name]); }
  buildLayersMenu();
}

function addAllLayers() {
  // Re-add after style change
  if (layers.lanes) loadLanes();
  if (layers.ports) loadPorts().then(() => { setupPortInteractions(); if (listOpen && listTab === 'ports') renderPortList(); });
  if (layers.airports && airportsData) { airportsData = null; loadAirports(); }
  // Arcs source
  map.addSource('arcs', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
  map.addLayer({ id: 'arcs', type: 'line', source: 'arcs', paint: { 'line-color': '#00e5ff', 'line-width': 1.8, 'line-opacity': 0.7, 'line-dasharray': [2, 1] } });
}

// ── Map Load ──
map.on('load', () => { addAllLayers(); loadStats(); });

// ═══════════════════════════════════════════════════════════════
// INTERACTIONS
// ═══════════════════════════════════════════════════════════════
let popup = null;

function setupPortInteractions() {
  map.on('click', 'ports', async (e) => {
    const p = e.features[0].properties;
    showPortCard(p);
    map.flyTo({ center: e.lngLat, zoom: Math.max(map.getZoom(), 6), duration: 800 });
    // Fetch sea routes
    try {
      const sr = await fetch(API + '/v1/sea-routes/from/' + encodeURIComponent(p.name)).then(r => r.json());
      if (sr && sr.destinations) drawArcs(e.features[0].geometry.coordinates, sr.destinations, p.name);
      renderSeaRoutesInCard(sr);
    } catch (e) {}
  });
  map.on('mouseenter', 'ports', (e) => {
    map.getCanvas().style.cursor = 'pointer';
    const p = e.features[0].properties;
    popup = new maplibregl.Popup({ closeButton: false, offset: 10 })
      .setLngLat(e.lngLat)
      .setHTML(`<b>${p.name}</b> <span style="color:var(--text3)">(${p.port_size})</span><br><span style="font-size:11px;color:var(--text2)">${p.flag || ''} ${p.country}${p.locode ? ' · ' + p.locode : ''}</span>`)
      .addTo(map);
  });
  map.on('mouseleave', 'ports', () => { map.getCanvas().style.cursor = ''; if (popup) { popup.remove(); popup = null; } });
}

function setupAirportInteractions() {
  map.on('click', 'airports', (e) => {
    const p = e.features[0].properties;
    showAirportCard(p);
    map.flyTo({ center: e.lngLat, zoom: Math.max(map.getZoom(), 6), duration: 800 });
  });
  map.on('mouseenter', 'airports', (e) => {
    map.getCanvas().style.cursor = 'pointer';
    const p = e.features[0].properties;
    popup = new maplibregl.Popup({ closeButton: false, offset: 10 })
      .setLngLat(e.lngLat)
      .setHTML(`<b>${p.icao}</b>${p.iata ? ' / ' + p.iata : ''}<br><span style="font-size:11px;color:var(--text2)">${p.name}, ${p.country}</span>`)
      .addTo(map);
  });
  map.on('mouseleave', 'airports', () => { map.getCanvas().style.cursor = ''; if (popup) { popup.remove(); popup = null; } });
}

async function drawArcs(origin, destinations, portName) {
  // Use Dijkstra sea router for real curved paths
  const features = [];
  const dests = (destinations || []).slice(0, 20);
  const promises = dests.map(async d => {
    if (!portsData) return null;
    const dp = portsData.features.find(f => f.properties.name === d.destination);
    if (!dp) return null;
    const [dLon, dLat] = dp.geometry.coordinates;
    try {
      const resp = await fetch(`${API}/v1/sea-routes/route?from=${origin[1]},${origin[0]}&to=${dLat},${dLon}`);
      if (resp.ok) {
        const gj = await resp.json();
        gj.properties.dest = d.destination;
        gj.properties.distance = d.distance_nm;
        return gj;
      }
    } catch(e) {}
    // Fallback: straight line
    return { type: 'Feature', geometry: { type: 'LineString', coordinates: [origin, dp.geometry.coordinates] }, properties: { distance: d.distance_nm, dest: d.destination } };
  });
  const results = await Promise.all(promises);
  for (const f of results) if (f) features.push(f);
  const src = map.getSource('arcs');
  if (src) src.setData({ type: 'FeatureCollection', features });
}

function clearArcs() {
  const src = map.getSource('arcs');
  if (src) src.setData({ type: 'FeatureCollection', features: [] });
}

// ═══════════════════════════════════════════════════════════════
// CARDS
// ═══════════════════════════════════════════════════════════════
function openCard() { document.getElementById('pcard').classList.add('open'); }
function closeCard() { document.getElementById('pcard').classList.remove('open'); clearArcs(); }

function cardField(k, v) { return v ? `<div class="sf"><span class="k">${k}</span><span class="v">${v}</span></div>` : ''; }

function showPortCard(p) {
  document.getElementById('pcardIcon').textContent = '⚓';
  document.getElementById('pcardName').textContent = p.name;
  document.getElementById('pcardType').textContent = p.port_size;
  document.getElementById('pcardMeta').textContent = [p.country, p.locode, p.zone_code].filter(Boolean).join(' · ');
  document.getElementById('pcardBody').innerHTML = `
    <div class="sc-section"><div class="st">Port Details</div>
      ${cardField('Max Vessel', p.max_vessel_size)}${cardField('Channel Depth', p.channel_depth_m ? p.channel_depth_m + ' m' : '')}
      ${cardField('Cargo Depth', p.cargo_depth_m ? p.cargo_depth_m + ' m' : '')}${cardField('WPI ID', p.wpi_id)}
      ${cardField('LOCODE', p.locode)}${cardField('Zone', p.zone_code)}
    </div><div class="sc-section" id="seaRoutesSection"><div class="st">Sea Routes</div><div style="color:var(--text3);font-size:var(--fs-xs)">Loading...</div></div>`;
  const apiUrl = p.locode ? `/v1/ports/${p.locode}` : `/v1/ports/${p.wpi_id}`;
  document.getElementById('pcardActions').innerHTML = `<a class="act" href="${API}${apiUrl}" target="_blank">Try API</a><button class="act" onclick="viewJson('${apiUrl}')">View JSON</button>`;
  openCard();
}

function renderSeaRoutesInCard(sr) {
  const el = document.getElementById('seaRoutesSection');
  if (!el) return;
  if (!sr || !sr.destinations || sr.destinations.length === 0) { el.innerHTML = '<div class="st">Sea Routes</div><div style="color:var(--text3);font-size:var(--fs-xs)">No sea routes found</div>'; return; }
  const rows = sr.destinations.slice(0, 20).map(d => `<div class="route-row" onclick="flyToPort('${d.destination}')"><span class="dest">→ ${d.destination}</span><span class="dist">${d.distance_nm} nm</span></div>`).join('');
  el.innerHTML = `<div class="st">Sea Routes (${sr.destinations.length})</div>${rows}`;
}

function showAirportCard(p) {
  document.getElementById('pcardIcon').textContent = '✈';
  document.getElementById('pcardName').textContent = p.icao + (p.iata ? ' / ' + p.iata : '');
  document.getElementById('pcardType').textContent = p.route_count + ' routes';
  document.getElementById('pcardMeta').textContent = [p.name, p.city, p.country].filter(Boolean).join(', ');
  document.getElementById('pcardBody').innerHTML = `
    <div class="sc-section"><div class="st">Airport Details</div>
      ${cardField('ICAO', p.icao)}${cardField('IATA', p.iata)}${cardField('Name', p.name)}
      ${cardField('City', p.city)}${cardField('Country', p.country)}${cardField('Routes', p.route_count)}
    </div>`;
  document.getElementById('pcardActions').innerHTML = `<a class="act" href="${API}/v1/airports/${p.icao}" target="_blank">Try API</a><button class="act" onclick="viewJson('/v1/airports/${p.icao}')">View JSON</button>`;
  openCard();
}

function showShipCard(s) {
  document.getElementById('pcardIcon').textContent = '🚢';
  document.getElementById('pcardName').textContent = s.name || s.mmsi;
  document.getElementById('pcardType').textContent = s.ship_type ? 'Type ' + s.ship_type : '';
  document.getElementById('pcardMeta').textContent = [s.country, 'MMSI ' + s.mmsi].filter(Boolean).join(' · ');
  document.getElementById('pcardBody').innerHTML = `
    <div class="sc-section"><div class="st">Ship Details</div>
      ${cardField('MMSI', s.mmsi)}${cardField('Call Sign', s.call_sign)}${cardField('Name', s.name)}
      ${cardField('Country', s.country)}${cardField('Gross Tonnage', s.gross_tonnage ? s.gross_tonnage.toLocaleString() : '')}
      ${cardField('Length', s.length_m ? s.length_m + ' m' : '')}${cardField('Beam', s.beam_m ? s.beam_m + ' m' : '')}
      ${cardField('Ship Type', s.ship_type)}${cardField('Class', s.class)}
    </div>`;
  document.getElementById('pcardActions').innerHTML = `<a class="act" href="${API}/v1/ships/${s.mmsi}" target="_blank">Try API</a><button class="act" onclick="viewJson('/v1/ships/${s.mmsi}')">View JSON</button>`;
  openCard();
}

function showRouteCard(r) {
  document.getElementById('pcardIcon').textContent = '✈';
  document.getElementById('pcardName').textContent = r.callsign;
  document.getElementById('pcardType').textContent = r.airline_code;
  document.getElementById('pcardMeta').textContent = r.airport_codes;
  document.getElementById('pcardBody').innerHTML = `
    <div class="sc-section"><div class="st">Flight Route</div>
      ${cardField('Callsign', r.callsign)}${cardField('Airline', r.airline_code)}${cardField('Number', r.number)}
      ${cardField('Route', r.airport_codes)}
    </div>`;
  document.getElementById('pcardActions').innerHTML = `<a class="act" href="${API}/v1/routes/${r.callsign}" target="_blank">Try API</a><button class="act" onclick="viewJson('/v1/routes/${r.callsign}')">View JSON</button>`;
  openCard();
  // Draw arc if we have airport data
  if (airportsData && r.airport_codes) {
    const codes = r.airport_codes.split('-');
    if (codes.length === 2) {
      const a1 = airportsData.features.find(f => f.properties.icao === codes[0]);
      const a2 = airportsData.features.find(f => f.properties.icao === codes[1]);
      if (a1 && a2) {
        const src = map.getSource('arcs');
        if (src) src.setData({ type: 'FeatureCollection', features: [{ type: 'Feature', geometry: { type: 'LineString', coordinates: [a1.geometry.coordinates, a2.geometry.coordinates] }, properties: {} }] });
      }
    }
  }
}

async function viewJson(path) {
  try {
    const data = await fetch(API + path).then(r => r.json());
    document.getElementById('pcardBody').innerHTML = `<div class="sc-section"><div class="st">JSON Response</div><div class="code-block">${JSON.stringify(data, null, 2)}</div></div>`;
  } catch (e) {}
}

function flyToPort(name) {
  if (!portsData) return;
  const f = portsData.features.find(f => f.properties.name === name);
  if (f) map.flyTo({ center: f.geometry.coordinates, zoom: 7, duration: 1000 });
}

// ═══════════════════════════════════════════════════════════════
// RAIL & MENUS
// ═══════════════════════════════════════════════════════════════
function railToggle(id) {
  const el = document.getElementById(id);
  const isOpen = el.classList.contains('open');
  document.querySelectorAll('.rail-pop').forEach(p => p.classList.remove('open'));
  if (!isOpen) el.classList.add('open');
}
document.addEventListener('click', (e) => {
  if (!e.target.closest('.rail-item') && !e.target.closest('.rail-pop')) {
    document.querySelectorAll('.rail-pop').forEach(p => p.classList.remove('open'));
  }
});

function buildLayersMenu() {
  document.getElementById('layersPop').innerHTML = [
    { name: 'ports', label: 'Seaports', color: '#f97316', count: '3,630', type: 'dot' },
    { name: 'lanes', label: 'Shipping Lanes', color: '#ef4444', count: '3 tiers', type: 'line' },
    { name: 'airports', label: 'Airports', color: '#60a5fa', count: '3,862', type: 'dot' },
  ].map(l => `<label class="set-toggle"><span class="layer-row"><span class="layer-${l.type}" style="background:${l.color}"></span><span class="layer-label">${l.label}</span><span class="layer-count">${l.count}</span></span><span class="switch"><input type="checkbox" ${layers[l.name] ? 'checked' : ''} onchange="toggleLayer('${l.name}')"><span class="track"></span></span></label>`).join('');
}

function buildMapMenu() {
  document.getElementById('mapPop').innerHTML = Object.keys(MAP_STYLES).map(k =>
    `<button class="pop-item${k === curStyle ? ' active' : ''}" onclick="switchStyle('${k}')">${k}</button>`).join('');
}

const REGIONS = [
  { name: 'Global', center: [20, 20], zoom: 2 },
  { name: 'Europe', center: [10, 50], zoom: 4 },
  { name: 'Asia-Pacific', center: [110, 15], zoom: 3 },
  { name: 'Americas', center: [-80, 20], zoom: 3 },
  { name: 'Middle East', center: [50, 25], zoom: 4 },
  { name: 'Oceania', center: [150, -25], zoom: 4 },
];
function buildRegionMenu() {
  document.getElementById('regionPop').innerHTML = REGIONS.map(r =>
    `<button class="pop-item" onclick="map.flyTo({center:${JSON.stringify(r.center)},zoom:${r.zoom},duration:1200});railToggle('regionPop')">${r.name}</button>`).join('');
}

function buildSettingsMenu() {
  document.getElementById('settingsPop').innerHTML = `
    <a class="pop-item" href="https://github.com/ngtrthanh/hpr-traffic-api" target="_blank">GitHub</a>
    <a class="pop-item" href="${API}/v1/mcp" target="_blank">MCP Manifest</a>
    <a class="pop-item" href="${API}/v1/stats" target="_blank">API Stats</a>`;
}

buildLayersMenu(); buildMapMenu(); buildRegionMenu(); buildSettingsMenu();

// ═══════════════════════════════════════════════════════════════
// LIST PANEL
// ═══════════════════════════════════════════════════════════════
let listOpen = false, listTab = 'ports';

function toggleList() {
  const el = document.getElementById('vlist');
  const api = document.getElementById('apiguide');
  listOpen = !listOpen;
  el.classList.toggle('open', listOpen);
  api.classList.remove('open');
  document.getElementById('railListBtn').classList.toggle('on', listOpen);
  document.getElementById('railApiBtn').classList.remove('on');
  if (listOpen) renderList();
}

function setListTab(tab) {
  listTab = tab;
  document.querySelectorAll('#vlist .vtab').forEach(t => t.classList.toggle('on', t.dataset.tab === tab));
  renderList();
}

function renderList() {
  const el = document.getElementById('vlistRows');
  if (!el) return;
  if (listTab === 'ports') renderPortList(el);
  else if (listTab === 'airports') renderAirportList(el);
  else renderAirlineList(el);
}

function renderPortList(el) {
  if (!el) el = document.getElementById('vlistRows');
  if (!el) return;
  if (!portsData) { el.innerHTML = '<div class="vlist-empty">Loading ports...</div>'; return; }
  const sorted = [...portsData.features].sort((a, b) => a.properties.name.localeCompare(b.properties.name));
  el.innerHTML = sorted.slice(0, 200).map(f => {
    const p = f.properties;
    const col = { Major: '#f97316', Large: '#facc15', Medium: '#22d3ee' }[p.port_size] || '#94a3b8';
    return `<div class="vrow" onclick="portRowClick('${p.name}')"><span class="vdot" style="background:${col}"></span><div class="vmain"><div class="vname">${p.name}</div><div class="vmeta">${p.country}${p.locode ? ' · ' + p.locode : ''}</div></div></div>`;
  }).join('');
}

function renderAirportList(el) {
  if (!airportsData) { el.innerHTML = '<div class="vlist-empty">Enable Airports layer first</div>'; return; }
  const sorted = [...airportsData.features].sort((a, b) => (b.properties.route_count || 0) - (a.properties.route_count || 0));
  el.innerHTML = sorted.slice(0, 200).map(f => {
    const p = f.properties;
    return `<div class="vrow" onclick="airportRowClick('${p.icao}')"><span class="vdot" style="background:#60a5fa"></span><div class="vmain"><div class="vname">${p.icao}${p.iata ? ' / ' + p.iata : ''}</div><div class="vmeta">${p.name}, ${p.country} · ${p.route_count || 0} routes</div></div></div>`;
  }).join('');
}

function renderAirlineList(el) {
  el.innerHTML = '<div class="vlist-empty">Use search to find flights by callsign</div>';
}

function portRowClick(name) {
  const f = portsData.features.find(f => f.properties.name === name);
  if (f) { map.flyTo({ center: f.geometry.coordinates, zoom: 7, duration: 800 }); showPortCard(f.properties); }
}
function airportRowClick(icao) {
  if (!airportsData) return;
  const f = airportsData.features.find(f => f.properties.icao === icao);
  if (f) { map.flyTo({ center: f.geometry.coordinates, zoom: 7, duration: 800 }); showAirportCard(f.properties); }
}

// ═══════════════════════════════════════════════════════════════
// API GUIDE
// ═══════════════════════════════════════════════════════════════
let guideTab = 'quickstart';

function toggleApi() {
  const el = document.getElementById('apiguide');
  const vl = document.getElementById('vlist');
  const isOpen = el.classList.contains('open');
  el.classList.toggle('open', !isOpen);
  vl.classList.remove('open'); listOpen = false;
  document.getElementById('railApiBtn').classList.toggle('on', !isOpen);
  document.getElementById('railListBtn').classList.remove('on');
  if (!isOpen) renderGuide();
}

function setGuideTab(tab) {
  guideTab = tab;
  document.querySelectorAll('#apiguide .vtab').forEach(t => t.classList.toggle('on', t.dataset.tab === tab));
  renderGuide();
}

const GUIDE = {
  quickstart: `<h3>HPR Traffic API</h3><p>Base URL: <code>https://traffic.hpradar.com</code></p><h3>Try it</h3>
<div class="code-block">curl /v1/routes/BAW123
curl /v1/ports/NLRTM
curl /v1/ships/353800000
curl /v1/sea-routes/from/Singapore</div>
<h3>Datasets</h3><p>507k flight routes · 3,630 seaports · 746k ships · 8k airports · 23k sea distances · shipping lanes</p>
<p>Pure Go, zero deps, in-memory, 18k req/sec.</p>`,

  endpoints: `<h3>Aviation</h3><div class="endpoint-group">
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/routes/{callsign}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/airlines/{icao}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/airports/{icao}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v0/aircraft/{mode_s}</span></div></div>
<h3>Maritime</h3><div class="endpoint-group">
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/ports/{locode}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/ports/nearby?lat=&lon=&radius_km=</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/ships/{mmsi}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/sea-routes/from/{port}</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/shipping-lanes</span></div></div>
<h3>Batch</h3><div class="endpoint-group">
<div class="endpoint-row"><span class="method">POST</span><span class="path">/v1/batch/routes</span></div>
<div class="endpoint-row"><span class="method">POST</span><span class="path">/v1/batch/ships</span></div></div>
<h3>Meta</h3><div class="endpoint-group">
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/stats</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/mcp</span></div></div>`,

  mcp: `<h3>MCP Server</h3><p>Connect Claude, Cursor, or any MCP-compatible AI agent:</p>
<div class="code-block">URL: https://traffic.hpradar.com/v1/mcp</div>
<h3>Available Tools</h3>
<div class="tool-row"><b>lookup_flight_route</b> — callsign → origin/dest</div>
<div class="tool-row"><b>lookup_aircraft</b> — Mode-S/reg → details</div>
<div class="tool-row"><b>lookup_ship</b> — MMSI/callsign → ship data</div>
<div class="tool-row"><b>lookup_port</b> — LOCODE/WPI → port info</div>
<div class="tool-row"><b>sea_distance</b> — port → all distances</div>
<div class="tool-row"><b>nearby_ports</b> — lat/lon/radius → ports</div>
<div class="tool-row"><b>search_sea_routes</b> — query → port names</div>
<h3>Example Call</h3>
<div class="code-block">POST /v1/mcp/call
{"tool":"lookup_port","parameters":{"id":"NLRTM"}}</div>`,

  code: `<h3>JavaScript</h3>
<div class="code-block">const res = await fetch('https://traffic.hpradar.com/v1/routes/BAW123');
const route = await res.json();
console.log(route.airport_codes); // "EGLL-OTHH"</div>
<h3>Python</h3>
<div class="code-block">import requests
r = requests.get('https://traffic.hpradar.com/v1/ports/NLRTM')
port = r.json()
print(port['name'], port['port_size'])  # ROTTERDAM Major</div>
<h3>curl</h3>
<div class="code-block">curl -s https://traffic.hpradar.com/v1/ships/353800000 | jq .name
# "EVER BREED"</div>
<h3>Batch (POST)</h3>
<div class="code-block">curl -X POST https://traffic.hpradar.com/v1/batch/routes \\
  -d '{"callsigns":["BAW123","RYR1234","EZY567"]}'</div>`,

  binary: `<h3>HPR-Atlas Binary Protocol</h3>
<p>~10x smaller than JSON. Zero-parse on client via <code>DataView</code>. 60 FPS canvas rendering.</p>
<h3>Endpoints</h3><div class="endpoint-group">
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/ports/bin</span></div>
<div class="endpoint-row"><span class="method">GET</span><span class="path">/v1/airports/bin</span></div></div>
<h3>Frame Layout (16B header)</h3>
<div class="code-block">Bytes  Field
0-3    Magic "HPRA"
4      Version (1)
5      Type (1=ports, 2=airports)
6-7    Point count (uint16 LE)
8-11   String table offset (uint32 LE)
12-15  Reserved</div>
<h3>Port Point (16B each)</h3>
<div class="code-block">0-3    Latitude  (int32, × 1e-6)
4-7    Longitude (int32, × 1e-6)
8      Size (0=VSmall..5=Major)
9-10   Name index (uint16)
11-12  Country index (uint16)
13     Flags (bit0=has locode)
14-15  TEU thousands (uint16)</div>
<h3>JS Parsing</h3>
<div class="code-block">const buf = await fetch('/v1/ports/bin').then(r=>r.arrayBuffer());
const dv = new DataView(buf);
const count = dv.getUint16(6, true);
const stOff = dv.getUint32(8, true);
for (let i=0; i&lt;count; i++) {
  const off = 16 + i*16;
  const lat = dv.getInt32(off, true) / 1e6;
  const lon = dv.getInt32(off+4, true) / 1e6;
  const size = dv.getUint8(off+8);
  const teu = dv.getUint16(off+14, true);
}</div>
<h3>Performance</h3>
<p>Ports: <b>103 KB</b> binary vs 1,014 KB JSON (9.8× smaller)<br>
Parse: <b>&lt;1ms</b> DataView vs ~15ms JSON.parse<br>
Zero GC pressure, constant 60 FPS canvas</p>`
};

function renderGuide() { document.getElementById('guideContent').innerHTML = GUIDE[guideTab] || ''; }

// ═══════════════════════════════════════════════════════════════
// SEARCH
// ═══════════════════════════════════════════════════════════════
let searchTimeout;
function onSearchInput() {
  clearTimeout(searchTimeout);
  const q = document.getElementById('searchInput').value.trim();
  if (q.length < 2) { hideSearchResults(); return; }
  searchTimeout = setTimeout(() => doSearch(q), 300);
}
function onSearchKey(e) { if (e.key === 'Escape') { hideSearchResults(); document.getElementById('searchInput').blur(); } }
function hideSearchResults() { document.getElementById('searchResults').style.display = 'none'; }

async function doSearch(q) {
  const results = [];
  // MMSI (9+ digits)
  if (/^\d{5,}$/.test(q)) {
    try { const s = await fetch(API + '/v1/ships/' + q).then(r => r.ok ? r.json() : null); if (s) results.push({ icon: '🚢', text: `${s.name || q} (${s.mmsi})`, sub: s.country, action: () => showShipCard(s) }); } catch (e) {}
  }
  // Callsign
  if (/^[A-Za-z]{2,4}\d/i.test(q)) {
    try { const r = await fetch(API + '/v1/routes/' + q.toUpperCase()).then(r => r.ok ? r.json() : null); if (r) results.push({ icon: '✈', text: `${r.callsign}: ${r.airport_codes}`, sub: r.airline_code, action: () => showRouteCard(r) }); } catch (e) {}
  }
  // Port name search
  try {
    const ports = await fetch(API + '/v1/sea-routes/search?q=' + encodeURIComponent(q)).then(r => r.ok ? r.json() : []);
    if (ports) ports.slice(0, 5).forEach(name => results.push({ icon: '⚓', text: name, sub: 'Sea route port', action: () => flyToPort(name) }));
  } catch (e) {}

  const el = document.getElementById('searchResults');
  if (results.length === 0) { el.innerHTML = '<div class="sr-item" style="color:var(--text3)">No results</div>'; }
  else { el.innerHTML = results.map((r, i) => `<div class="sr-item" onmousedown="searchAction(${i})"><span class="sr-icon">${r.icon}</span><div class="vmain"><div class="vname">${r.text}</div><div class="vmeta">${r.sub || ''}</div></div></div>`).join(''); }
  el.style.display = 'block';
  window._searchResults = results;
}
function searchAction(i) { const r = window._searchResults[i]; if (r && r.action) r.action(); hideSearchResults(); }

// ═══════════════════════════════════════════════════════════════
// STATS
// ═══════════════════════════════════════════════════════════════
async function loadStats() {
  try {
    const s = await fetch(API + '/v1/stats').then(r => r.json());
    document.getElementById('sRoutes').textContent = (s.aviation?.routes || 0).toLocaleString();
    document.getElementById('sPorts').textContent = (s.maritime?.seaports || 0).toLocaleString();
    document.getElementById('sShips').textContent = (s.maritime?.ships || 0).toLocaleString();
  } catch (e) {}
}

// Open list on desktop
if (window.innerWidth > 700) { toggleList(); }

// ═══════════════════════════════════════════════════════════════
// DRAG & PIN
// ═══════════════════════════════════════════════════════════════
const DRAG_STORE_KEY = 'hpr_pinned_positions';
const PIN_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 17v5"/><path d="M9 11V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v7"/><path d="M5 11h14l-1.5 6h-11z"/></svg>';
const GRIP_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="9" cy="6" r="1"/><circle cx="15" cy="6" r="1"/><circle cx="9" cy="12" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="18" r="1"/><circle cx="15" cy="18" r="1"/></svg>';

function getSavedPositions() {
  try { return JSON.parse(localStorage.getItem(DRAG_STORE_KEY) || '{}'); } catch { return {}; }
}
function savePosition(id, x, y) {
  const p = getSavedPositions(); p[id] = { x, y }; localStorage.setItem(DRAG_STORE_KEY, JSON.stringify(p));
}
function removePosition(id) {
  const p = getSavedPositions(); delete p[id]; localStorage.setItem(DRAG_STORE_KEY, JSON.stringify(p));
}

function makeDraggable(el) {
  const id = el.id;
  if (el.classList.contains('draggable')) return; // already initialized
  el.classList.add('draggable');

  // Insert toolbar at top
  const toolbar = document.createElement('div');
  toolbar.className = 'panel-toolbar drag-handle';
  toolbar.innerHTML = GRIP_SVG + '<span class="spacer"></span>';
  const pinBtn = document.createElement('button');
  pinBtn.className = 'pin-btn';
  pinBtn.title = 'Pin position';
  pinBtn.innerHTML = PIN_SVG;
  pinBtn.onmousedown = e => e.stopPropagation();
  pinBtn.onclick = () => togglePin(el, pinBtn);
  toolbar.appendChild(pinBtn);
  el.insertBefore(toolbar, el.firstChild);

  // Restore pinned position
  const saved = getSavedPositions()[id];
  if (saved) applyPosition(el, saved.x, saved.y, pinBtn);

  // Drag logic
  let startX, startY, elX, elY;
  toolbar.addEventListener('mousedown', e => {
    if (e.target.closest('.pin-btn')) return;
    e.preventDefault();
    const rect = el.getBoundingClientRect();
    const parent = el.offsetParent.getBoundingClientRect();
    elX = rect.left - parent.left;
    elY = rect.top - parent.top;
    startX = e.clientX;
    startY = e.clientY;
    document.addEventListener('mousemove', onDrag);
    document.addEventListener('mouseup', onDragEnd);
  });

  function onDrag(e) {
    const dx = e.clientX - startX, dy = e.clientY - startY;
    const nx = elX + dx, ny = elY + dy;
    el.classList.add('dragged');
    el.style.left = nx + 'px';
    el.style.top = ny + 'px';
    el.style.right = 'auto';
  }
  function onDragEnd() {
    document.removeEventListener('mousemove', onDrag);
    document.removeEventListener('mouseup', onDragEnd);
    // If pinned, save new position
    if (el.classList.contains('pinned')) {
      savePosition(id, parseInt(el.style.left), parseInt(el.style.top));
    }
  }
}

function applyPosition(el, x, y, pinBtn) {
  el.classList.add('dragged', 'pinned');
  el.style.left = x + 'px';
  el.style.top = y + 'px';
  el.style.right = 'auto';
  if (pinBtn) pinBtn.classList.add('pinned');
}

function togglePin(el, btn) {
  if (el.classList.contains('pinned')) {
    // Unpin: remove saved, reset to CSS defaults
    el.classList.remove('pinned', 'dragged');
    el.style.left = '';
    el.style.top = '';
    el.style.right = '';
    btn.classList.remove('pinned');
    removePosition(el.id);
  } else {
    // Pin current position
    el.classList.add('pinned');
    btn.classList.add('pinned');
    if (!el.classList.contains('dragged')) {
      // Save current rendered position
      const rect = el.getBoundingClientRect();
      const parent = el.offsetParent.getBoundingClientRect();
      const x = rect.left - parent.left, y = rect.top - parent.top;
      el.classList.add('dragged');
      el.style.left = x + 'px';
      el.style.top = y + 'px';
      el.style.right = 'auto';
    }
    savePosition(el.id, parseInt(el.style.left), parseInt(el.style.top));
  }
}

// Init draggable on panels
document.addEventListener('DOMContentLoaded', () => {
  ['rail', 'vlist', 'apiguide', 'pcard'].forEach(id => {
    const el = document.getElementById(id);
    if (el) makeDraggable(el);
  });
});
