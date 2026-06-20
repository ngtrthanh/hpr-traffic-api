#!/usr/bin/env python3
"""
Enrich ships in SQLite with photos from builder/operator fleet pages.

Strategy: builders and operators publish fleet galleries with professional photos.
We match by IMO/ship name and store the photo URL.

Sources (all public marketing pages):
  - Evergreen: evergreen-marine.com/fleet
  - MSC: msc.com/our-fleet
  - CMA CGM: cma-cgm.com/the-group/fleet
  - Samsung Heavy Industries: samsungshi.com
  - Lürssen: cdn.lurssen.com (done)
  - Hyundai Heavy: hhi.co.kr
  - DSME: dsme.co.kr

Usage: python3 scripts/enrich-photos.py
"""
import sqlite3, urllib.request, re, json, time, os

DB_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'data', 'enrichment.db')

def get_db():
    return sqlite3.connect(DB_PATH)

def enrich_evergreen(db):
    """Evergreen publishes vessel specs at shipmentlink.com with photos."""
    # Evergreen vessel names start with "EVER "
    ships = db.execute("SELECT imo, name FROM ships WHERE name LIKE 'EVER %' AND photo1 = ''").fetchall()
    if not ships:
        return 0
    # Wikimedia Commons has most Evergreen ships
    count = 0
    for imo, name in ships[:50]:
        # Pattern: Ever_Given_(ship,_2018)_001.jpg
        clean = name.title().replace(' ', '_')
        # Try common Wikimedia patterns
        url = f"https://commons.wikimedia.org/wiki/Special:FilePath/{clean}_(ship).jpg"
        try:
            req = urllib.request.Request(url, method='HEAD')
            resp = urllib.request.urlopen(req, timeout=5)
            if resp.status == 200:
                db.execute("UPDATE ships SET photo1=?, source='wikimedia' WHERE imo=?", (url, imo))
                count += 1
                time.sleep(0.3)
        except:
            pass
    db.commit()
    return count

def enrich_msc(db):
    """MSC ships: try Wikimedia by name pattern MSC_{Name}."""
    ships = db.execute("SELECT imo, name FROM ships WHERE name LIKE 'MSC %' AND photo1 = '' LIMIT 50").fetchall()
    count = 0
    for imo, name in ships:
        clean = name.title().replace(' ', '_')
        url = f"https://commons.wikimedia.org/wiki/Special:FilePath/{clean}.jpg"
        try:
            req = urllib.request.Request(url, method='HEAD')
            resp = urllib.request.urlopen(req, timeout=5)
            if resp.status == 200:
                db.execute("UPDATE ships SET photo1=?, source='wikimedia' WHERE imo=?", (url, imo))
                count += 1
                time.sleep(0.3)
        except:
            pass
    db.commit()
    return count

def enrich_from_wikimedia_by_imo(db, limit=100):
    """Try Wikimedia Commons search by IMO number for any ship without photo."""
    ships = db.execute(
        "SELECT imo, name FROM ships WHERE photo1 = '' AND imo != '' AND gt > 50000 ORDER BY gt DESC LIMIT ?",
        (limit,)
    ).fetchall()
    count = 0
    for imo, name in ships:
        # Try common naming patterns on Wikimedia
        patterns = [
            f"{name.title().replace(' ', '_')}_(ship).jpg",
            f"{name.title().replace(' ', '_')}.jpg",
            f"{name.replace(' ', '_')}.jpg",
        ]
        for pat in patterns:
            url = f"https://commons.wikimedia.org/wiki/Special:FilePath/{pat}"
            try:
                req = urllib.request.Request(url, method='HEAD')
                req.add_header('User-Agent', 'HPRadar-Enrichment/1.0')
                resp = urllib.request.urlopen(req, timeout=5)
                if resp.status == 200:
                    db.execute("UPDATE ships SET photo1=?, source='wikimedia' WHERE imo=?", (url, imo))
                    count += 1
                    break
            except:
                pass
            time.sleep(0.2)
    db.commit()
    return count

def main():
    db = get_db()
    total = db.execute("SELECT COUNT(*) FROM ships WHERE photo1 = ''").fetchone()[0]
    print(f"Ships without photos: {total}")
    
    print("\n[1/3] Enriching Evergreen fleet from Wikimedia...")
    n = enrich_evergreen(db)
    print(f"  Added: {n}")
    
    print("[2/3] Enriching MSC fleet from Wikimedia...")
    n = enrich_msc(db)
    print(f"  Added: {n}")
    
    print("[3/3] Enriching top ships by GT from Wikimedia...")
    n = enrich_from_wikimedia_by_imo(db, limit=50)
    print(f"  Added: {n}")
    
    remaining = db.execute("SELECT COUNT(*) FROM ships WHERE photo1 = ''").fetchone()[0]
    print(f"\nRemaining without photos: {remaining}")
    db.close()

if __name__ == '__main__':
    main()
