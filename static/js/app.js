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
      const cc = countryRaw || '';
      const country = cc;
      const flag = cc ? `<span class="fi fi-${cc.toLowerCase()}"></span>` : '';
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

  // CIA lanes: clean global view (low zoom) — Major red, Middle amber, Minor gray
  map.addLayer({ id: 'lanes-t1', type: 'line', source: 'lanes', filter: ['==', ['get', 'tier'], 1], maxzoom: 7,
    paint: { 'line-color': '#dc2626', 'line-width': ['interpolate', ['linear'], ['zoom'], 1, 0.8, 4, 1.2, 6, 1.5], 'line-opacity': 0.55 } });
  map.addLayer({ id: 'lanes-t2', type: 'line', source: 'lanes', filter: ['==', ['get', 'tier'], 2], maxzoom: 7,
    paint: { 'line-color': '#d97706', 'line-width': ['interpolate', ['linear'], ['zoom'], 1, 0.5, 4, 0.8, 6, 1], 'line-opacity': 0.4 } });
  map.addLayer({ id: 'lanes-t3', type: 'line', source: 'lanes', filter: ['==', ['get', 'tier'], 3], maxzoom: 7,
    paint: { 'line-color': '#6b7280', 'line-width': ['interpolate', ['linear'], ['zoom'], 1, 0.3, 4, 0.5, 6, 0.7], 'line-opacity': 0.25 } });

  // Marnet detail: only at z5+ (progressive)
  map.addLayer({ id: 'lanes-t4', type: 'line', source: 'lanes', filter: ['==', ['get', 'tier'], 4], minzoom: 5,
    paint: { 'line-color': '#0ea5e9', 'line-width': ['interpolate', ['linear'], ['zoom'], 5, 0.2, 8, 0.5, 11, 0.8], 'line-opacity': ['interpolate', ['linear'], ['zoom'], 5, 0.1, 8, 0.2, 11, 0.35] } });

  // Top-30 port hub links
  if (portsData) {
    const top30 = portsData.features.filter(f => f.properties.teu > 0)
      .sort((a,b) => b.properties.teu - a.properties.teu).slice(0, 30);
    const hubFeats = [];
    for (let i = 0; i < top30.length; i++) {
      for (let j = i+1; j < top30.length; j++) {
        const [lon1,lat1] = top30[i].geometry.coordinates;
        const [lon2,lat2] = top30[j].geometry.coordinates;
        const d = Math.abs(lon2-lon1) + Math.abs(lat2-lat1);
        if (d < 5 || d > 200) continue; // skip too close or antipodal
        hubFeats.push({ type:'Feature', geometry:{ type:'LineString', coordinates:[[lon1,lat1],[lon2,lat2]] }, properties:{} });
      }
    }
    if (hubFeats.length) {
      map.addSource('hub-lanes', { type:'geojson', data:{ type:'FeatureCollection', features: hubFeats } });
      map.addLayer({ id:'lanes-hubs', type:'line', source:'hub-lanes', maxzoom: 6,
        paint:{ 'line-color':'#f97316', 'line-width': ['interpolate',['linear'],['zoom'], 1, 0.4, 4, 0.8], 'line-opacity': 0.3, 'line-dasharray': [4, 3] } });
    }
  }
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
      const nameRaw = strings[nameIdx] || '';
      const [name, cc] = nameRaw.includes('|') ? nameRaw.split('|') : [nameRaw, ''];
      const flag = cc ? `<span class="fi fi-${cc.toLowerCase()}"></span>` : '';
      features.push({ type: 'Feature', geometry: { type: 'Point', coordinates: [lon, lat] }, properties: {
        name, icao: strings[icaoIdx] || '', iata: strings[iataIdx] || '', route_count: routeCount, country_code: cc, flag
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
  if (id === 'lanes') { ['lanes-t1', 'lanes-t2', 'lanes-t3', 'lanes-t4', 'lanes-hubs'].forEach(l => { if (map.getLayer(l)) map.setLayoutProperty(l, 'visibility', v); }); }
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
map.on('load', () => { addAllLayers(); loadStats(); showIntroCard(); });

// ═══════════════════════════════════════════════════════════════
// INTERACTIONS
// ═══════════════════════════════════════════════════════════════
let popup = null;

function setupPortInteractions() {
  map.on('click', 'ports', async (e) => {
    const p = e.features[0].properties;
    showPortCard(p);
    map.flyTo({ center: e.lngLat, zoom: Math.max(map.getZoom(), 6), duration: 800 });
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
      .setHTML(`<b>${p.icao}</b>${p.iata ? ' / ' + p.iata : ''}<br><span style="font-size:11px;color:var(--text2)">${p.flag || ''} ${p.name}</span>`)
      .addTo(map);
  });
  map.on('mouseleave', 'airports', () => { map.getCanvas().style.cursor = ''; if (popup) { popup.remove(); popup = null; } });
}

function drawArcs(origin, destinations, portName) {
  const features = [];
  for (const d of (destinations || []).slice(0, 40)) {
    if (!portsData) break;
    const dp = portsData.features.find(f => f.properties.name === d.destination);
    if (!dp) continue;
    features.push({ type: 'Feature', geometry: { type: 'LineString', coordinates: [origin, dp.geometry.coordinates] }, properties: { distance: d.distance_nm, dest: d.destination } });
  }
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

function showPortCard(p) { showEntityCard('port', p); }

function showAirportCard(p) { showEntityCard('airport', p); }

function showShipCard(s) { showEntityCard('ship', s); }

function showRouteCard(r) { showEntityCard('route', r); }

// ═══════════════════════════════════════════════════════════════
// UNIFIED VCARD — schema-driven entity renderer
// ═══════════════════════════════════════════════════════════════
const ENTITY_SCHEMA = {
  port: {
    icon: '⚓', label: d => d.name,
    subtitle: d => d.port_size || 'Seaport',
    meta: d => [d.flag, d.country, d.locode, d.zone_code].filter(Boolean).join(' · '),
    identity: d => ({key: 'locode', value: d.locode}),
    sections: [
      {title: 'Port Details', fields: d => [
        ['Country', (d.flag||'') + ' ' + (d.country||'')], ['LOCODE', d.locode], ['Zone', d.zone_code],
        ['TEU Throughput', d.teu_thousands ? d.teu_thousands.toLocaleString()+'k TEU' : ''],
        ['Max Vessel', d.max_vessel_size],
        ['Channel Depth', d.channel_depth_m ? d.channel_depth_m+' m' : ''],
        ['Cargo Depth', d.cargo_depth_m ? d.cargo_depth_m+' m' : ''],
      ]},
    ],
    actions: d => {
      const apiUrl = d.locode ? `/v1/ports/${d.locode}` : `/v1/ports/${encodeURIComponent(d.name)}`;
      return `<button class="act" onclick="togglePortRoutes('${d.name.replace(/'/g,"\\'")}', this)">Show Routes</button><a class="act" href="${API}${apiUrl}" target="_blank">API</a><button class="act" onclick="toggleJson('${apiUrl}', this)">JSON</button>`;
    },
    photo: d => (d.teu_thousands > 500 || d.port_size === 'Major'),
    enrich: enrichPort,
  },
  airport: {
    icon: '✈', label: d => d.icao + (d.iata ? ' / ' + d.iata : ''),
    subtitle: d => (d.route_count || 0) + ' flights',
    meta: d => [d.flag, d.name, d.city].filter(Boolean).join(' · '),
    identity: d => ({key: 'icao', value: d.icao}),
    sections: [
      {title: 'Airport Details', fields: d => [
        ['Country', (d.flag||'') + ' ' + (d.country_code||'')],
        ['ICAO', d.icao], ['IATA', d.iata], ['Name', d.name], ['City', d.city],
        ['Flights', d.route_count],
      ]},
    ],
    actions: d => `<button class="act" onclick="toggleAirRoutes('${d.icao}', this)">Show Routes</button><a class="act" href="${API}/v1/airports/${d.icao}" target="_blank">API</a><button class="act" onclick="toggleJson('/v1/airports/${d.icao}', this)">JSON</button>`,
  },
  ship: {
    icon: '🚢', label: d => d.name || d.mmsi,
    subtitle: d => d.ship_type ? 'Type ' + d.ship_type : '',
    meta: d => {
      const f = d.country_code ? `<span class="fi fi-${d.country_code.toLowerCase()}"></span>` : '';
      return [f, d.country, 'MMSI ' + d.mmsi].filter(Boolean).join(' · ');
    },
    identity: d => ({key: 'mmsi', value: d.mmsi}),
    sections: d => {
      const s = [];
      if (d.operator) s.push({title: 'Operator', fields: () => [
        ['Company', (d.operator.country_code ? '<span class="fi fi-'+d.operator.country_code.toLowerCase()+'"></span> ' : '') + d.operator.name],
        ['Sector', d.operator.sector],
      ]});
      s.push({title: 'Ship Details', fields: () => [
        ['MMSI', d.mmsi], ['Call Sign', d.call_sign], ['Name', d.name],
        ['Country', (d.country_code ? '<span class="fi fi-'+d.country_code.toLowerCase()+'"></span> ' : '') + (d.country||'')],
        ['Gross Tonnage', d.gross_tonnage ? d.gross_tonnage.toLocaleString() : ''],
        ['Length', d.length_m ? d.length_m+' m' : ''], ['Beam', d.beam_m ? d.beam_m+' m' : ''],
        ['Ship Type', d.ship_type], ['Class', d.class],
      ]});
      return s;
    },
    actions: d => `<a class="act" href="${API}/v1/ships/${d.mmsi}" target="_blank">API</a><button class="act" onclick="viewJson('/v1/ships/${d.mmsi}')">JSON</button>`,
    dynamic: {title: 'Live Tracking', placeholder: '📡 No live data — connect AIS feed', fields: ['Position','Speed','Heading','Destination','ETA','Draught']},
  },
  route: {
    icon: '✈', label: d => d.callsign,
    subtitle: d => d.airline_code,
    meta: d => d.airport_codes,
    identity: d => ({key: 'callsign', value: d.callsign}),
    sections: [
      {title: 'Flight Route', fields: d => [
        ['Callsign', d.callsign], ['Airline', d.airline_code], ['Number', d.number], ['Route', d.airport_codes],
      ]},
    ],
    actions: d => `<a class="act" href="${API}/v1/routes/${d.callsign}" target="_blank">API</a><button class="act" onclick="viewJson('/v1/routes/${d.callsign}')">JSON</button>`,
    dynamic: {title: 'Live Flight', placeholder: '📡 No live data — connect ADS-B feed', fields: ['Position','Altitude','Speed','Heading','Squawk']},
  },
  company: {
    icon: '🏢', label: d => d.name,
    subtitle: d => d.sector,
    meta: d => {
      const f = d.country_code ? `<span class="fi fi-${d.country_code.toLowerCase()}"></span>` : '';
      return [f, d.full_name].filter(Boolean).join(' ');
    },
    identity: d => ({key: 'imo_company', value: d.imo_company || d.code}),
    sections: [
      {title: 'Company Details', fields: d => [
        ['Code', d.code], ['IMO Company', d.imo_company],
        ['Country', (d.country_code ? '<span class="fi fi-'+d.country_code.toLowerCase()+'"></span> ' : '') + (d.country_code||'')],
        ['Sector', d.sector], ['Parent', d.parent], ['Fleet', d.fleet_size + ' ships'],
        ['TEU Capacity', d.teu_capacity ? (d.teu_capacity/1000).toFixed(0)+'k TEU' : ''],
        ['Website', d.website ? `<a href="https://${d.website}" target="_blank">${d.website}</a>` : ''],
      ]},
    ],
    actions: d => `<a class="act" href="${API}/v1/companies" target="_blank">API</a>`,
    dynamic: {title: 'Fleet Status', placeholder: '📡 No live fleet data', fields: ['At Sea','In Port','Anchored']},
  },
};

function showEntityCard(type, data) {
  const schema = ENTITY_SCHEMA[type];
  if (!schema) return;
  document.getElementById('pcardIcon').textContent = schema.icon;
  document.getElementById('pcardName').textContent = schema.label(data);
  document.getElementById('pcardType').textContent = schema.subtitle(data);
  document.getElementById('pcardMeta').innerHTML = schema.meta(data);

  // Build sections
  const sections = typeof schema.sections === 'function' ? schema.sections(data) : schema.sections;
  let body = '';

  // Photo placeholder for ports
  if (schema.photo && schema.photo(data)) {
    const pid = 'entity-photo-' + Date.now();
    body += `<div class="sc-section" style="padding:0;overflow:hidden;border-radius:var(--r-lg) var(--r-lg) 0 0"><div id="${pid}" style="height:100px;background:linear-gradient(135deg,var(--surface-2) 0%,var(--border) 100%);display:flex;align-items:center;justify-content:center;color:var(--text3);font-size:32px;position:relative;overflow:hidden">${schema.icon}</div></div>`;
    data._photoId = pid;
  }

  // Static sections
  for (const sec of sections) {
    const fields = (typeof sec.fields === 'function' ? sec.fields(data) : sec.fields)
      .filter(([,v]) => v).map(([k,v]) => cardField(k, v)).join('');
    if (fields) body += `<div class="sc-section"><div class="st">${sec.title}</div>${fields}</div>`;
  }

  // Dynamic placeholder section (for tracker integration)
  if (schema.dynamic) {
    const id = data.identity || schema.identity(data);
    body += `<div class="sc-section sc-dynamic" data-entity-type="${type}" data-entity-key="${id.value}"><div class="st">${schema.dynamic.title}</div><div class="dynamic-placeholder" style="font-size:11px;color:var(--text3);padding:4px 0">${schema.dynamic.placeholder}</div></div>`;
  }

  document.getElementById('pcardBody').innerHTML = body;
  document.getElementById('pcardActions').innerHTML = schema.actions(data);
  openCard();

  // Post-render enrichment
  if (schema.enrich) schema.enrich(data);

  // Draw arc for routes
  if (type === 'route' && airportsData && data.airport_codes) {
    const codes = data.airport_codes.split('-');
    if (codes.length === 2) {
      const a1 = airportsData.features.find(f => f.properties.icao === codes[0]);
      const a2 = airportsData.features.find(f => f.properties.icao === codes[1]);
      if (a1 && a2) {
        const src = map.getSource('arcs');
        if (src) src.setData({type:'FeatureCollection',features:[{type:'Feature',geometry:{type:'LineString',coordinates:[a1.geometry.coordinates,a2.geometry.coordinates]},properties:{}}]});
      }
    }
  }
}

// Inject live data into card's dynamic section (called by tracker when data arrives)
function updateEntityLive(type, key, liveData) {
  const el = document.querySelector(`.sc-dynamic[data-entity-type="${type}"][data-entity-key="${key}"]`);
  if (!el) return;
  const fields = Object.entries(liveData).filter(([,v]) => v != null).map(([k,v]) => cardField(k, v)).join('');
  el.innerHTML = `<div class="st">${ENTITY_SCHEMA[type].dynamic.title}</div>${fields || '<div class="dynamic-placeholder" style="font-size:11px;color:var(--text3)">Waiting for data...</div>'}`;
}

// Port enrichment (async fetch full details + photo)
function enrichPort(data) {
  const apiUrl = data.locode ? `/v1/ports/${data.locode}` : `/v1/ports/${encodeURIComponent(data.name)}`;
  fetch(API + apiUrl).then(r => r.ok ? r.json() : null).then(full => {
    if (!full) return;
    const body = document.getElementById('pcardBody');
    if (!body) return;
    const section = body.querySelector('.sc-section:nth-child(2)');
    if (!section) return;
    let extra = '';
    if (full.function) {
      const fn = full.function, services = [];
      if (fn[0]==='1') services.push('Seaport');
      if (fn[1]==='2') services.push('Rail terminal');
      if (fn[2]==='3') services.push('Road terminal');
      if (fn[3]==='4') services.push('Airport');
      if (fn[4]==='5') services.push('Postal');
      if (fn[5]==='6') services.push('Multimodal');
      if (services.length) extra += cardField('Services', services.join(', '));
    }
    if (full.status) {
      const sm = {AI:'Approved (international)',AA:'Approved (national)',RL:'Recognized',QQ:'Not verified',NGA:'NGA source'};
      extra += cardField('Status', sm[full.status] || full.status);
    }
    if (extra) section.innerHTML += extra;
    // Photo
    if (data._photoId && (data.teu_thousands > 500 || data.port_size === 'Major')) {
      const photoEl = document.getElementById(data._photoId);
      if (photoEl) {
        const wikiName = (data.name.charAt(0) + data.name.slice(1).toLowerCase()).replace(/ /g, '_');
        const img = new Image();
        img.style = 'width:100%;height:100%;object-fit:cover;opacity:0.8';
        img.src = `https://commons.wikimedia.org/wiki/Special:FilePath/Port_of_${wikiName}.jpg`;
        img.onload = () => { photoEl.innerHTML = ''; photoEl.appendChild(img); };
      }
    }
  }).catch(() => {});
}
  const rows = sr.destinations.slice(0, 20).map(d => `<div class="route-row" onclick="flyToPort('${d.destination}')"><span class="dest">→ ${d.destination}</span><span class="dist">${d.distance_nm} nm</span></div>`).join('');
  el.innerHTML = `<div class="st">Sea Routes (${sr.destinations.length})</div>${rows}`;
}

function showAirportCard(p) {
  document.getElementById('pcardIcon').textContent = '✈';
  document.getElementById('pcardName').textContent = p.icao + (p.iata ? ' / ' + p.iata : '');
  document.getElementById('pcardType').textContent = p.route_count + ' flights';
  document.getElementById('pcardMeta').innerHTML = [p.flag, p.name, p.city].filter(Boolean).join(' · ');
  document.getElementById('pcardBody').innerHTML = `
    <div class="sc-section"><div class="st">Airport Details</div>
      ${cardField('Country', (p.flag || '') + ' ' + (p.country_code || ''))}
      ${cardField('ICAO', p.icao)}${cardField('IATA', p.iata)}
      ${cardField('Name', p.name)}${cardField('City', p.city)}
      ${cardField('Flights', p.route_count)}
    </div>`;
  document.getElementById('pcardActions').innerHTML = `<button class="act" onclick="toggleAirRoutes('${p.icao}', this)">Show Routes</button><a class="act" href="${API}/v1/airports/${p.icao}" target="_blank">API</a><button class="act" onclick="toggleJson('/v1/airports/${p.icao}', this)">JSON</button>`;
  openCard();
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
  else if (listTab === 'ships') renderShipList(el);
  else if (listTab === 'operators') renderOperatorList(el);
  else renderAirlineList(el);
}

function renderPortList(el) {
  if (!portsData) { el.innerHTML = '<div class="vlist-empty">Loading ports...</div>'; return; }
  const cols = [{key:'name',label:'Port',w:'40%'},{key:'country',label:'Country',w:'20%'},{key:'port_size',label:'Size',w:'15%'},{key:'locode',label:'LOCODE',w:'25%'}];
  const rows = portsData.features.map(f => f.properties);
  renderVTable(el, cols, rows, vtableSort.ports || {key:'name',asc:true}, 'ports', r => portRowClick(r.name));
}

function renderAirportList(el) {
  if (!airportsData) { el.innerHTML = '<div class="vlist-empty">Enable Airports layer first</div>'; return; }
  const cols = [{key:'icao',label:'ICAO',w:'15%'},{key:'name',label:'Name',w:'35%'},{key:'country_code',label:'CC',w:'10%'},{key:'route_count',label:'Flights',w:'15%',num:true},{key:'iata',label:'IATA',w:'12%'}];
  const rows = airportsData.features.map(f => f.properties);
  renderVTable(el, cols, rows, vtableSort.airports || {key:'route_count',asc:false}, 'airports', r => airportRowClick(r.icao));
}

function renderAirlineList(el) {
  el.innerHTML = '<div class="vlist-empty">Use search to find flights by callsign</div>';
}

async function renderShipList(el) {
  if (!window._notableShips) {
    el.innerHTML = '<div class="vlist-empty">Loading ships...</div>';
    try {
      window._notableShips = await fetch(API + '/v1/ships/notable').then(r => r.json());
    } catch(e) { el.innerHTML = '<div class="vlist-empty">Failed to load</div>'; return; }
  }
  const data = window._notableShips;
  const cols = [{key:'name',label:'Name',w:'30%'},{key:'flag',label:'Flag',w:'10%'},{key:'ship_type',label:'Type',w:'15%'},{key:'dwt',label:'DWT',w:'15%',num:true,fmt:v=>v?(v/1000).toFixed(0)+'k':''},{key:'operator',label:'Operator',w:'20%'}];
  renderVTable(el, cols, data, vtableSort.ships || {key:'dwt',asc:false}, 'ships', r => {
    if (r.mmsi) fetch(API+'/v1/ships/'+r.mmsi).then(x=>x.json()).then(s=>showEntityCard('ship',s)).catch(()=>{});
    else showEntityCard('ship', r);
  });
}

async function renderOperatorList(el) {
  try {
    const data = await fetch(API + '/v1/companies').then(r => r.json());
    if (!data || !data.length) { el.innerHTML = '<div class="vlist-empty">No operators</div>'; return; }
    const cols = [{key:'name',label:'Operator',w:'30%'},{key:'country_code',label:'CC',w:'10%'},{key:'sector',label:'Sector',w:'12%'},{key:'fleet_size',label:'Fleet',w:'12%',num:true},{key:'teu_capacity',label:'TEU',w:'18%',num:true,fmt:v=>v?(v/1000).toFixed(0)+'k':''}];
    renderVTable(el, cols, data, vtableSort.operators || {key:'teu_capacity',asc:false}, 'operators', r => showEntityCard('company', r));
  } catch(e) { el.innerHTML = '<div class="vlist-empty">Failed to load</div>'; }
}

// ═══════════════════════════════════════════════════════════════
// VTABLE — unified sortable table renderer
// ═══════════════════════════════════════════════════════════════
const vtableSort = {};

function renderVTable(el, cols, rows, sort, tabKey, onRowClick) {
  // Sort
  const sorted = [...rows].sort((a, b) => {
    let va = a[sort.key], vb = b[sort.key];
    if (sort.num || typeof va === 'number') { va = va || 0; vb = vb || 0; return sort.asc ? va - vb : vb - va; }
    va = (va || '').toString().toLowerCase(); vb = (vb || '').toString().toLowerCase();
    return sort.asc ? va.localeCompare(vb) : vb.localeCompare(va);
  });

  // Header
  const hdr = cols.map(c => {
    const arrow = sort.key === c.key ? (sort.asc ? ' ▲' : ' ▼') : '';
    return `<th style="width:${c.w}" onclick="vtableSortBy('${tabKey}','${c.key}',${!!c.num})">${c.label}${arrow}</th>`;
  }).join('');

  // Rows (limit 200)
  const tbody = sorted.slice(0, 200).map(r => {
    const flag = r.country_code ? `<span class="fi fi-${r.country_code.toLowerCase()}"></span> ` : (r.flag || '');
    const cells = cols.map(c => {
      let v = r[c.key];
      if (c.fmt) v = c.fmt(v);
      else if (c.key === 'name' || c.key === 'country') v = flag + (v || '');
      else if (c.num && v) v = Number(v).toLocaleString();
      return `<td>${v || ''}</td>`;
    }).join('');
    return `<tr class="vrow-tr">${cells}</tr>`;
  }).join('');

  el.innerHTML = `<table class="vtable"><thead><tr>${hdr}</tr></thead><tbody>${tbody}</tbody></table>`;

  // Click handlers
  el.querySelectorAll('.vrow-tr').forEach((tr, i) => {
    tr.onclick = () => onRowClick(sorted[i]);
  });
}

function vtableSortBy(tabKey, key, isNum) {
  const cur = vtableSort[tabKey];
  if (cur && cur.key === key) cur.asc = !cur.asc;
  else vtableSort[tabKey] = {key, asc: !isNum, num: isNum};
  renderList();
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
  // Route search: "X to Y" or "X → Y" or "X > Y"
  const routeMatch = q.match(/^(.+?)\s*(?:to|→|>|->)\s*(.+)$/i);
  if (routeMatch) {
    const [, fromQ, toQ] = routeMatch;
    await searchRoute(fromQ.trim(), toQ.trim());
    return;
  }
  const results = [];
  // MMSI (9+ digits)
  if (/^\d{5,}$/.test(q)) {
    try { const s = await fetch(API + '/v1/ships/' + q).then(r => r.ok ? r.json() : null); if (s) { const sf = s.country_code ? `<span class="fi fi-${s.country_code.toLowerCase()}"></span> ` : ''; results.push({ icon: '🚢', text: `${s.name || q} (${s.mmsi})`, sub: sf + (s.country || ''), action: () => showShipCard(s) }); } } catch (e) {}
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

async function searchRoute(fromQ, toQ) {
  // Try sea first, then air
  let data = null, mode = 'sea';
  try {
    const resp = await fetch(`${API}/v1/routes/sea?from=${encodeURIComponent(fromQ)}&to=${encodeURIComponent(toQ)}`);
    if (resp.ok) data = await resp.json();
  } catch(e) {}
  if (!data || !data.properties) {
    try {
      const resp = await fetch(`${API}/v1/routes/air?from=${encodeURIComponent(fromQ)}&to=${encodeURIComponent(toQ)}`);
      if (resp.ok) { data = await resp.json(); mode = 'air'; }
    } catch(e) {}
  }
  if (!data || !data.properties) {
    const el = document.getElementById('searchResults');
    el.innerHTML = '<div class="sr-item" style="color:var(--text3)">No route found</div>';
    el.style.display = 'block';
    return;
  }
  hideSearchResults();
  plotRoute(data, mode);
}

function plotRoute(geojson, mode) {
  const p = geojson.properties;
  // Plot line on map
  if (map.getSource('route-line')) { map.getSource('route-line').setData(geojson); }
  else {
    map.addSource('route-line', { type: 'geojson', data: geojson });
    map.addLayer({ id: 'route-line', type: 'line', source: 'route-line',
      paint: { 'line-color': mode === 'sea' ? '#10b981' : '#a78bfa', 'line-width': 3, 'line-opacity': 0.85 } });
  }
  // Update line color for mode
  map.setPaintProperty('route-line', 'line-color', mode === 'sea' ? '#10b981' : '#a78bfa');
  // Fit bounds
  const coords = geojson.geometry.coordinates;
  const bounds = coords.reduce((b, c) => b.extend(c), new maplibregl.LngLatBounds(coords[0], coords[0]));
  map.fitBounds(bounds, { padding: 60, duration: 1000 });
  // Show route card
  showRouteInfoCard(p, mode);
}

function showRouteInfoCard(p, mode) {
  const icon = mode === 'sea' ? '⚓' : '✈';
  const fromFlag = p.from.country_code ? `<span class="fi fi-${p.from.country_code.toLowerCase()}"></span>` : '';
  const toFlag = p.to.country_code ? `<span class="fi fi-${p.to.country_code.toLowerCase()}"></span>` : '';
  const fromCode = p.from.code || p.from.iata || '';
  const toCode = p.to.code || p.to.iata || '';
  const days = p.estimated_hours >= 48 ? `${(p.estimated_hours/24).toFixed(1)} days` : `${p.estimated_hours}h`;

  document.getElementById('pcardIcon').textContent = icon;
  document.getElementById('pcardName').innerHTML = `${fromCode} → ${toCode}`;
  document.getElementById('pcardType').textContent = mode === 'sea' ? 'Sea Route' : 'Air Route';
  document.getElementById('pcardMeta').innerHTML = `${fromFlag} ${p.from.name} → ${toFlag} ${p.to.name}`;
  let body = `<div class="sc-section"><div class="st">Route Details</div>
    ${cardField('Distance', `${p.distance_nm.toLocaleString()} nm / ${p.distance_km.toLocaleString()} km`)}
    ${cardField('ETA', `${days} at ${p.speed_kn} kn`)}`;
  if (p.passes && p.passes.length) {
    body += cardField('Transits', p.passes.map(s => s.charAt(0).toUpperCase() + s.slice(1)).join(' → '));
  }
  if (p.airlines && p.airlines.length) {
    body += cardField('Airlines', p.airlines.join(', '));
  }
  body += `</div>
    <div class="sc-section"><div class="st">Route Progress</div>
      <div style="display:flex;justify-content:space-between;align-items:center;margin:8px 0">
        <div style="text-align:center"><div style="font-size:18px;font-weight:700">${fromCode}</div><div style="font-size:10px;color:var(--text3)">${p.from.name}</div></div>
        <div style="flex:1;margin:0 12px;border-top:1px dashed var(--border);position:relative;display:flex;align-items:center;justify-content:center">
          <span style="background:var(--bg2);padding:0 6px;font-size:11px;color:var(--text2)">${p.distance_nm.toLocaleString()} nm</span>
        </div>
        <div style="text-align:center"><div style="font-size:18px;font-weight:700">${toCode}</div><div style="font-size:10px;color:var(--text3)">${p.to.name}</div></div>
      </div>
      <div style="background:var(--bg3);border-radius:4px;height:6px;overflow:hidden"><div style="width:100%;height:100%;background:${mode==='sea'?'#10b981':'#a78bfa'};border-radius:4px"></div></div>
    </div>`;
  document.getElementById('pcardBody').innerHTML = body;
  document.getElementById('pcardActions').innerHTML = `<button class="act" onclick="clearRoute()">Clear Route</button>`;
  openCard();
}

function clearRoute() {
  if (map.getSource('route-line')) map.getSource('route-line').setData({ type: 'FeatureCollection', features: [] });
  if (map.getSource('air-lanes-src')) map.getSource('air-lanes-src').setData({ type: 'FeatureCollection', features: [] });
  closeCard();
}

async function drawAirLanes(icao) {
  const data = await fetch(`${API}/v1/air-lanes/geojson?icao=${icao}&limit=100`).then(r => r.json());
  if (!data || !data.features) return;
  if (map.getSource('air-lanes-src')) { map.getSource('air-lanes-src').setData(data); }
  else {
    map.addSource('air-lanes-src', { type: 'geojson', data });
    map.addLayer({ id: 'air-lanes-layer', type: 'line', source: 'air-lanes-src',
      paint: { 'line-color': '#a78bfa', 'line-width': 1.5, 'line-opacity': 0.6 } });
  }
}

function toggleAirRoutes(icao, btn) {
  const src = map.getSource('air-lanes-src');
  if (src && btn.dataset.active) {
    src.setData({ type: 'FeatureCollection', features: [] });
    btn.textContent = 'Show Routes';
    delete btn.dataset.active;
  } else {
    drawAirLanes(icao);
    btn.textContent = 'Hide Routes';
    btn.dataset.active = '1';
  }
}

async function togglePortRoutes(name, btn) {
  const body = document.getElementById('pcardBody');
  let section = body && body.querySelector('.sc-sea-routes');
  if (btn.dataset.active) {
    if (section) section.remove();
    clearArcs();
    btn.textContent = 'Show Routes';
    delete btn.dataset.active;
  } else {
    btn.textContent = 'Hide Routes';
    btn.dataset.active = '1';
    if (!section) {
      section = document.createElement('div');
      section.className = 'sc-section sc-sea-routes';
      section.innerHTML = '<div class="st">Sea Routes</div><div style="color:var(--text3);font-size:var(--fs-xs)">Loading...</div>';
      if (body) body.appendChild(section);
    }
    try {
      const sr = await fetch(API + '/v1/sea-routes/from/' + encodeURIComponent(name)).then(r => r.json());
      if (sr && sr.destinations) {
        const port = portsData.features.find(f => f.properties.name === name);
        if (port) drawArcs(port.geometry.coordinates, sr.destinations, name);
        const dests = sr.destinations.slice(0, 10).map(d => `<div class="sf"><span class="k">${d.destination}</span><span class="v">${d.distance_nm} nm</span></div>`).join('');
        section.innerHTML = `<div class="st">Sea Routes (${sr.destinations.length})</div>${dests}`;
      } else {
        section.innerHTML = '<div class="st">Sea Routes</div><div style="color:var(--text3);font-size:var(--fs-xs)">No routes found</div>';
      }
    } catch(e) { section.innerHTML = '<div class="st">Sea Routes</div><div style="color:var(--text3);font-size:var(--fs-xs)">Error loading routes</div>'; }
  }
}

async function toggleJson(url, btn) {
  const existing = document.getElementById('jsonPreview');
  if (existing) { existing.remove(); btn.textContent = 'JSON'; return; }
  btn.textContent = 'Close JSON';
  const data = await fetch(API + url).then(r => r.json());
  const pre = document.createElement('div');
  pre.id = 'jsonPreview';
  pre.className = 'sc-section';
  pre.innerHTML = `<pre style="font-size:10px;overflow:auto;max-height:200px;background:var(--surface-2);padding:8px;border-radius:4px;margin:0">${JSON.stringify(data, null, 2)}</pre>`;
  document.getElementById('pcardBody').appendChild(pre);
}

// ═══════════════════════════════════════════════════════════════
// STATS
// ═══════════════════════════════════════════════════════════════
async function loadStats() {
  try {
    const s = await fetch(API + '/v1/stats').then(r => r.json());
    document.getElementById('sPorts').textContent = (s.maritime?.seaports || 0).toLocaleString();
    document.getElementById('sAirports').textContent = (s.aviation?.airports || 0).toLocaleString();
    document.getElementById('sShips').textContent = (s.maritime?.ships || 0).toLocaleString();
    document.getElementById('sAircraft').textContent = (s.aviation?.aircraft || 0).toLocaleString();
    document.getElementById('sAirlines').textContent = (s.aviation?.airlines || 0).toLocaleString();
    document.getElementById('sCompanies').textContent = (s.maritime?.companies || 0).toLocaleString();
  } catch (e) {}
}

function showIntroCard() {
  const el = document.getElementById('introcard');
  if (!el) return;
  el.innerHTML = `
    <div class="sc-head"><div class="sc-title"><span>🌐</span><span class="sc-name">HPRadar Traffic</span><span class="sc-type" id="intro-version"></span></div><div class="sc-meta">Aviation & Maritime Data Explorer</div><button class="close btn-icon" onclick="document.getElementById('introcard').innerHTML=''" title="Close"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button></div>
    <div class="sc-body">
    <div class="sc-section">
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;margin:4px 0">
        <div style="background:var(--surface-2);border:1px solid var(--border);border-radius:6px;padding:8px 10px">
          <div style="font-size:12px;font-weight:700;margin-bottom:6px">🚢 Marine</div>
          <div style="font-size:11px;color:var(--text2);line-height:1.8" id="intro-marine">Loading...</div>
        </div>
        <div style="background:var(--surface-2);border:1px solid var(--border);border-radius:6px;padding:8px 10px">
          <div style="font-size:12px;font-weight:700;margin-bottom:6px">✈ Aviation</div>
          <div style="font-size:11px;color:var(--text2);line-height:1.8" id="intro-air">Loading...</div>
        </div>
      </div>
    </div>
    <div class="sc-section"><div class="st">Quick Start</div>
      <div style="font-size:11px;color:var(--text2);line-height:1.8">
        <b>Search:</b> "Singapore to Rotterdam" or "VVNB to KLAX"<br>
        <b>Click:</b> any port or airport for details<br>
        <b>Layers:</b> toggle map data<br>
        <b>List:</b> ports, operators, airports, airlines
      </div>
    </div>
    <div class="sc-section"><div class="st">Connections</div>
      <div style="font-size:11px;color:var(--text2);line-height:1.6">
        <span style="display:inline-block;background:var(--surface-2);border:1px solid var(--border);border-radius:4px;padding:1px 6px;margin:2px;font-size:10px">REST /v1/*</span>
        <span style="display:inline-block;background:var(--surface-2);border:1px solid var(--border);border-radius:4px;padding:1px 6px;margin:2px;font-size:10px">WebSocket /ws</span>
        <span style="display:inline-block;background:var(--surface-2);border:1px solid var(--border);border-radius:4px;padding:1px 6px;margin:2px;font-size:10px">Binary HPRA</span>
        <span style="display:inline-block;background:var(--surface-2);border:1px solid var(--border);border-radius:4px;padding:1px 6px;margin:2px;font-size:10px">MCP</span>
        <span style="display:inline-block;background:var(--surface-2);border:1px solid var(--border);border-radius:4px;padding:1px 6px;margin:2px;font-size:10px">GeoJSON</span>
      </div>
    </div>
    <div class="sc-section">
      <div style="font-size:10px;color:var(--text3);line-height:1.5">
        <b>Credits:</b> adsbdb · OurAirports · VRS · NGA WPI/PUB151 · ITU List V · CIA/Paul Benden · eurostat · World Bank CPPI · HPRadar · flag-icons
      </div>
    </div>
    <div class="sc-section">
      <div style="font-size:11px;color:var(--text2);line-height:1.6;text-align:center">
        ⭐ Open source — <a href="https://github.com/ngtrthanh/hpr-traffic-api" target="_blank" style="color:var(--accent);font-weight:600">Star · Fork · PR</a>
      </div>
    </div>
    </div>
    <div class="sc-actions"><a class="act" href="https://github.com/ngtrthanh/hpr-traffic-api" target="_blank">⭐ GitHub</a><a class="act" href="${API}/v1/stats" target="_blank">API Stats</a><a class="act" href="https://hpradar.com" target="_blank">HPRadar</a></div>`;
  // Populate stats from API
  fetch(API + '/v1/stats').then(r => r.json()).then(s => {
    const fmt = n => n >= 1000000 ? (n/1000000).toFixed(0)+'M' : n >= 1000 ? (n/1000).toFixed(1).replace(/\.0$/,'')+'k' : n;
    const v = document.getElementById('intro-version');
    if (v && s.version) v.textContent = s.version;
    const m = document.getElementById('intro-marine');
    const a = document.getElementById('intro-air');
    if (m) m.innerHTML = `<div>${fmt(s.maritime?.ships||0)} vessels</div><div>${fmt(s.maritime?.seaports||0)} seaports</div><div>${fmt(s.maritime?.companies||0)} operators</div><div>Dijkstra routing</div><div>29.5k sea lanes</div>`;
    if (a) a.innerHTML = `<div>${fmt(s.aviation?.aircraft||0)} aircraft</div><div>${fmt(s.aviation?.airports||0)} airports</div><div>${fmt(s.aviation?.routes||0)} flight routes</div><div>${fmt(s.aviation?.airlines||0)} airlines</div><div>Multi-hop graph</div>`;
  }).catch(() => {});
}

// List stays closed until user opens it

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
  // Add close button for pcard
  if (id === 'pcard') {
    const closeBtn = document.createElement('button');
    closeBtn.className = 'pin-btn';
    closeBtn.title = 'Close';
    closeBtn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';
    closeBtn.onmousedown = e => e.stopPropagation();
    closeBtn.onclick = () => closeCard();
    toolbar.appendChild(closeBtn);
  }
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
