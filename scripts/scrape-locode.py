#!/usr/bin/env python3
"""Download and parse UN/LOCODE from official MDB bulk file (cleanest source)."""
import subprocess, csv, os, io, re, urllib.request, zipfile

URL = "https://service.unece.org/trade/locode/loc242mdb.zip"
CACHE = "/tmp/locode.zip"
MDB_DIR = "/tmp/locode_mdb"
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

def main():
    # Download if not cached
    if not os.path.exists(CACHE):
        print(f"Downloading {URL}...")
        urllib.request.urlretrieve(URL, CACHE)
    
    # Extract MDB
    os.makedirs(MDB_DIR, exist_ok=True)
    with zipfile.ZipFile(CACHE) as zf:
        zf.extractall(MDB_DIR)
    
    # Find the .mdb file
    mdb = None
    for f in os.listdir(MDB_DIR):
        if f.endswith('.mdb'):
            mdb = os.path.join(MDB_DIR, f)
            break
    if not mdb:
        print("ERROR: No .mdb file found")
        return

    # Export table via mdbtools
    print(f"Exporting from: {mdb}")
    table = subprocess.check_output(["mdb-tables", "-1", mdb]).decode().strip().split('\n')
    main_table = [t for t in table if 'CodeList' in t and 'Country' not in t][0]
    print(f"  Table: {main_table}")
    
    raw = subprocess.check_output(["mdb-export", mdb, main_table]).decode('latin-1', errors='replace')
    reader = csv.DictReader(io.StringIO(raw))
    
    ports = []
    for row in reader:
        country = row.get('Country', '').strip()
        location = row.get('Location', '').strip()
        name = row.get('Name', '').strip() or row.get('NameWoDiacritics', '').strip()
        function = row.get('Function', '').strip()
        status = row.get('Status', '').strip()
        coords = row.get('Coordinates', '').strip()

        if not country or not location or not name:
            continue
        # Only seaports: function position 1 = port
        if not function or (len(function) > 0 and function[0] != '1' and '1' not in function[:1]):
            # Check if first char is '1' (seaport)
            if len(function) < 1 or function[0] != '1':
                continue

        locode = country + location
        lat, lon = parse_coords(coords)
        ports.append({
            'locode': locode,
            'name': name,
            'country_code': country,
            'function': function,
            'lat': lat or '',
            'lon': lon or '',
            'status': status,
        })

    # Write output
    with open(OUTPUT, 'w', newline='') as f:
        w = csv.DictWriter(f, fieldnames=['locode', 'name', 'country_code', 'function', 'lat', 'lon', 'status'])
        w.writeheader()
        w.writerows(ports)

    print(f"\nDone: {len(ports)} seaports written to {OUTPUT}")

if __name__ == '__main__':
    main()

