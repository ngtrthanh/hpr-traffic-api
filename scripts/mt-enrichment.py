#!/usr/bin/env python3
"""
MT Enrichment: merge MarineTraffic export into notable_ships.csv + SQLite.

Expects CSV with columns (flexible naming):
  flag, name, imo, type, mmsi, year_built, length, width, dwt, gt, teu, builder, operator

Usage:
  1. Export from MT: flag,shipname,imo,specific_ship_type,mmsi,year_of_build,length,width,dwt,gross_tonnage,teu,ship_builder,operator
  2. Save to data/imports/ (any .csv filename)
  3. Run: python3 scripts/mt-enrichment.py
"""
import csv, os, sys, re, sqlite3, glob

BASE = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
IMPORT_DIR = os.path.join(BASE, 'data', 'imports')
NOTABLE_CSV = os.path.join(BASE, 'data', 'notable_ships.csv')
DB_PATH = os.path.join(BASE, 'data', 'enrichment.db')

# Essential output fields
FIELDS = ['imo', 'mmsi', 'name', 'flag', 'ship_type', 'dwt', 'gt', 'teu',
           'length_m', 'beam_m', 'year_built', 'builder', 'operator',
           'sector', 'status', 'photo1', 'photo2']

# Column name mapping (MT variations → our field)
COL_MAP = {
    'imo': 'imo', 'imo number': 'imo',
    'mmsi': 'mmsi',
    'shipname': 'name', 'ship name': 'name', 'vessel name': 'name', 'name': 'name',
    'flag': 'flag',
    'specific ship type': 'ship_type', 'specific_ship_type': 'ship_type',
    'ship type': 'ship_type', 'vessel type - generic': 'ship_type', 'vessel type': 'ship_type',
    'year of build': 'year_built', 'year_of_build': 'year_built', 'built': 'year_built', 'year built': 'year_built',
    'length': 'length_m', 'length overall': 'length_m', 'loa': 'length_m',
    'width': 'beam_m', 'beam': 'beam_m', 'breadth': 'beam_m',
    'dwt': 'dwt', 'capacity - dwt': 'dwt', 'deadweight': 'dwt',
    'gross tonnage': 'gt', 'gross_tonnage': 'gt', 'gt': 'gt',
    'teu': 'teu', 'teu capacity': 'teu',
    'ship builder': 'builder', 'ship_builder': 'builder', 'builder': 'builder',
    'operator': 'operator',
}

SECTOR_MAP = {
    'container': 'CS', 'container ship': 'CS', 'fully cellular': 'CS',
    'bulk': 'BC', 'ore': 'BC', 'capesize': 'BC', 'panamax': 'BC',
    'tanker': 'TK', 'vlcc': 'TK', 'suezmax': 'TK', 'aframax': 'TK', 'crude': 'TK', 'chemical': 'TK', 'product': 'TK',
    'lng': 'LNG', 'lpg': 'LNG',
    'car carrier': 'CC', 'vehicles': 'CC', 'pctc': 'CC',
    'ro-ro': 'RR', 'roro': 'RR',
    'cruise': 'PAX', 'passenger': 'PAX',
    'yacht': 'YAC', 'sailing': 'YAC',
    'offshore': 'OFF', 'supply': 'OFF', 'fpso': 'OFF', 'drill': 'OFF',
    'cargo': 'CARGO',
}

def detect_sector(t):
    if not t: return ''
    tl = t.lower()
    for k, v in SECTOR_MAP.items():
        if k in tl: return v
    return 'CARGO'

def num(s):
    if not s: return ''
    s = str(s).replace(',', '').strip()
    m = re.search(r'[\d.]+', s)
    return m.group(0) if m else ''

def init_db():
    db = sqlite3.connect(DB_PATH)
    db.execute('''CREATE TABLE IF NOT EXISTS ships (
        imo TEXT PRIMARY KEY, mmsi TEXT, name TEXT, flag TEXT, ship_type TEXT,
        dwt INTEGER, gt INTEGER, teu INTEGER, length_m REAL, beam_m REAL,
        year_built INTEGER, builder TEXT, operator TEXT, sector TEXT,
        photo1 TEXT, photo2 TEXT, updated TEXT DEFAULT CURRENT_TIMESTAMP
    )''')
    db.execute('CREATE INDEX IF NOT EXISTS idx_ships_mmsi ON ships(mmsi)')
    db.execute('CREATE INDEX IF NOT EXISTS idx_ships_name ON ships(name)')
    db.commit()
    return db

def main():
    # Find import files
    csvs = glob.glob(os.path.join(IMPORT_DIR, '*.csv'))
    if not csvs:
        print(f"No CSV files in {IMPORT_DIR}")
        sys.exit(1)

    # Init SQLite
    db = init_db()
    print(f"SQLite: {DB_PATH}")

    # Load existing notable ships (for photo preservation)
    existing = {}
    if os.path.exists(NOTABLE_CSV):
        with open(NOTABLE_CSV) as f:
            for row in csv.DictReader(f):
                key = row.get('imo') or row.get('mmsi')
                if key: existing[key] = row

    total_loaded = 0
    for csv_path in csvs:
        print(f"\nProcessing: {os.path.basename(csv_path)}")
        with open(csv_path, encoding='utf-8-sig', errors='replace') as f:
            reader = csv.DictReader(f)
            col_map = {}
            for col in reader.fieldnames:
                k = col.lower().strip()
                if k in COL_MAP:
                    col_map[col] = COL_MAP[k]
            print(f"  Mapped {len(col_map)}/{len(reader.fieldnames)} columns")

            batch = []
            for row in reader:
                ship = {COL_MAP[k.lower().strip()]: row[k].strip() for k in row if k.lower().strip() in COL_MAP}
                imo = num(ship.get('imo', ''))
                mmsi = num(ship.get('mmsi', ''))
                if not imo and not mmsi: continue

                sector = detect_sector(ship.get('ship_type', ''))
                dwt = int(num(ship.get('dwt', '')) or 0)
                gt = int(num(ship.get('gt', '')) or 0)
                teu = int(num(ship.get('teu', '')) or 0)
                length = float(num(ship.get('length_m', '')) or 0)
                beam = float(num(ship.get('beam_m', '')) or 0)
                year = int(num(ship.get('year_built', '')) or 0)

                # Preserve existing photos
                photos = existing.get(imo, existing.get(mmsi, {}))

                batch.append((
                    imo, mmsi, ship.get('name', ''), ship.get('flag', ''),
                    ship.get('ship_type', ''), dwt, gt, teu, length, beam,
                    year, ship.get('builder', ''), ship.get('operator', ''),
                    sector, photos.get('photo1', ''), photos.get('photo2', '')
                ))

            # Upsert to SQLite
            db.executemany('''INSERT OR REPLACE INTO ships
                (imo, mmsi, name, flag, ship_type, dwt, gt, teu, length_m, beam_m,
                 year_built, builder, operator, sector, photo1, photo2)
                VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)''', batch)
            db.commit()
            total_loaded += len(batch)
            print(f"  Loaded: {len(batch)} ships")

    print(f"\nTotal in SQLite: {db.execute('SELECT COUNT(*) FROM ships').fetchone()[0]}")

    # Export to CSV
    rows = db.execute('SELECT * FROM ships ORDER BY gt DESC, dwt DESC').fetchall()
    cols = [d[0] for d in db.execute('SELECT * FROM ships LIMIT 0').description]
    with open(NOTABLE_CSV, 'w', newline='') as f:
        w = csv.writer(f)
        w.writerow(FIELDS)
        for row in rows:
            d = dict(zip(cols, row))
            w.writerow([d.get('imo',''), d.get('mmsi',''), d.get('name',''),
                       d.get('flag',''), d.get('ship_type',''),
                       d.get('dwt',''), d.get('gt',''), d.get('teu',''),
                       d.get('length_m',''), d.get('beam_m',''),
                       d.get('year_built',''), d.get('builder',''), d.get('operator',''),
                       d.get('sector',''), 'active',
                       d.get('photo1',''), d.get('photo2','')])

    print(f"Exported: {len(rows)} ships → {NOTABLE_CSV}")
    db.close()

if __name__ == '__main__':
    main()
