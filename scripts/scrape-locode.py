#!/usr/bin/env python3
"""Download and parse UN/LOCODE from official CSV bulk file."""
import urllib.request, zipfile, csv, os, io, re

URL = "https://service.unece.org/trade/locode/loc242csv.zip"
CACHE = "/tmp/locode_csv.zip"
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
        print(f"Downloaded: {os.path.getsize(CACHE)} bytes")
    else:
        print(f"Using cached: {CACHE}")

    # Extract and parse CSV files from zip
    # The zip contains files like 2024-2 UNLOCODE CodeListPart1.csv, Part2.csv, Part3.csv
    ports = []
    with zipfile.ZipFile(CACHE) as zf:
        for name in sorted(zf.namelist()):
            if not name.endswith('.csv'):
                continue
            print(f"  Parsing: {name}")
            with zf.open(name) as f:
                text = io.TextIOWrapper(f, encoding='latin-1', errors='replace')
                reader = csv.reader(text)
                for row in reader:
                    if len(row) < 12:
                        continue
                    # Columns: Change,Country,Location,Name,NameWoDiacritics,Subdivision,Function,Status,Date,IATA,Coordinates,Remarks
                    country = row[1].strip()
                    location = row[2].strip()
                    name_val = row[3].strip()
                    function = row[6].strip()
                    status = row[7].strip()
                    coords = row[10].strip()

                    if not country or not location or not name_val:
                        continue
                    # Only keep seaports (function bit 1 = port)
                    if not function or '1' not in function:
                        continue

                    locode = country + location
                    if len(locode) < 4:
                        continue

                    lat, lon = parse_coords(coords)
                    ports.append({
                        'locode': locode,
                        'name': name_val,
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

