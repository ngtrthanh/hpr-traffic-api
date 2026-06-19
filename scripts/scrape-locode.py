#!/usr/bin/env python3
"""Scrape UN/LOCODE seaport data from UNECE for all countries."""
import urllib.request, re, csv, time, os, sys

COUNTRIES = [
    "ad","ae","af","ag","ai","al","am","ao","ar","as","at","au","aw","az",
    "ba","bb","bd","be","bf","bg","bh","bi","bj","bm","bn","bo","br","bs",
    "bt","bw","by","bz","ca","cd","cf","cg","ch","ci","ck","cl","cm","cn",
    "co","cr","cu","cv","cw","cy","cz","de","dj","dk","dm","do","dz","ec",
    "ee","eg","er","es","et","fi","fj","fk","fm","fo","fr","ga","gb","gd",
    "ge","gf","gh","gi","gl","gm","gn","gp","gq","gr","gt","gu","gw","gy",
    "hk","hn","hr","ht","hu","id","ie","il","in","iq","ir","is","it","jm",
    "jo","jp","ke","kg","kh","ki","km","kn","kp","kr","kw","ky","kz","la",
    "lb","lc","lk","lr","ls","lt","lu","lv","ly","ma","mc","md","me","mg",
    "mh","mk","ml","mm","mn","mo","mq","mr","ms","mt","mu","mv","mw","mx",
    "my","mz","na","nc","ne","ng","ni","nl","no","np","nr","nz","om","pa",
    "pe","pf","pg","ph","pk","pl","pm","pr","ps","pt","pw","py","qa","re",
    "ro","rs","ru","rw","sa","sb","sc","sd","se","sg","sh","si","sk","sl",
    "sn","so","sr","ss","st","sv","sx","sy","sz","tc","td","tg","th","tj",
    "tl","tm","tn","to","tr","tt","tv","tw","tz","ua","ug","us","uy","uz",
    "vc","ve","vg","vi","vn","vu","wf","ws","ye","za","zm","zw",
]

URL = "https://service.unece.org/trade/locode/{}.htm"
CACHE_DIR = "/tmp/locode_cache"
OUTPUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "data", "locode_ports.csv")

def parse_coords(s):
    """Parse UNECE coord format like '2049N 10640E' to decimal."""
    if not s or len(s) < 5:
        return None, None
    m = re.match(r'(\d{2})(\d{2})([NS])\s*(\d{3,5})(\d{2})([EW])', s.strip())
    if not m:
        return None, None
    lat = int(m.group(1)) + int(m.group(2)) / 60
    if m.group(3) == 'S': lat = -lat
    lon = int(m.group(4)) + int(m.group(5)) / 60
    if m.group(6) == 'W': lon = -lon
    return round(lat, 4), round(lon, 4)

def scrape_country(cc):
    """Fetch and parse a single country page. Returns list of port dicts."""
    cache_file = os.path.join(CACHE_DIR, f"{cc}.htm")
    
    if os.path.exists(cache_file):
        with open(cache_file, 'r', errors='replace') as f:
            html = f.read()
    else:
        url = URL.format(cc)
        try:
            req = urllib.request.Request(url, headers={'User-Agent': 'HPRadar-LOCODE-Scraper/1.0'})
            with urllib.request.urlopen(req, timeout=15) as resp:
                html = resp.read().decode('latin-1', errors='replace')
            with open(cache_file, 'w') as f:
                f.write(html)
            time.sleep(0.5)
        except Exception as e:
            print(f"  SKIP {cc}: {e}", file=sys.stderr)
            return []

    # Parse table rows - UNECE uses simple HTML tables
    # Columns: Change | LOCODE | Name | NameWoDiacritics | Subdiv | Function | Status | Date | IATA | Coordinates | Remarks
    ports = []
    # Find all <tr> with data
    rows = re.findall(r'<tr[^>]*>(.*?)</tr>', html, re.DOTALL | re.IGNORECASE)
    for row in rows:
        cells = re.findall(r'<td[^>]*>(.*?)</td>', row, re.DOTALL | re.IGNORECASE)
        if len(cells) < 10:
            continue
        # Clean HTML tags and entities from cells
        clean = [re.sub(r'<[^>]+>', '', c).replace('&nbsp;', '').replace('&amp;', '&').strip() for c in cells]
        locode_raw = clean[1].strip()  # e.g., "VN HPH"
        name = clean[2].strip()
        function = clean[5].strip()
        status = clean[6].strip()
        coords = clean[9].strip()
        
        if not locode_raw or not name:
            continue
        # Only keep locations with port function (bit 1)
        if not function or '1' not in function:
            continue
        
        # Build proper LOCODE (remove space)
        locode = locode_raw.replace(' ', '')
        if len(locode) < 4:
            continue
            
        lat, lon = parse_coords(coords)
        ports.append({
            'locode': locode,
            'name': name,
            'country_code': cc.upper(),
            'function': function,
            'lat': lat or '',
            'lon': lon or '',
            'status': status,
        })
    return ports

def main():
    os.makedirs(CACHE_DIR, exist_ok=True)
    
    all_ports = []
    total = len(COUNTRIES)
    for i, cc in enumerate(COUNTRIES):
        ports = scrape_country(cc)
        all_ports.extend(ports)
        if ports:
            print(f"[{i+1}/{total}] {cc.upper()}: {len(ports)} seaports")
        else:
            print(f"[{i+1}/{total}] {cc.upper()}: 0")
    
    # Write output
    with open(OUTPUT, 'w', newline='') as f:
        w = csv.DictWriter(f, fieldnames=['locode', 'name', 'country_code', 'function', 'lat', 'lon', 'status'])
        w.writeheader()
        w.writerows(all_ports)
    
    print(f"\nDone: {len(all_ports)} seaports written to {OUTPUT}")

if __name__ == '__main__':
    main()
