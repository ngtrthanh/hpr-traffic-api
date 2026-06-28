#!/usr/bin/env python3
"""Merge new aircraft from Mictronics database into aircraft.csv"""
import csv, sys, os

AIRCRAFT_CSV = os.path.join(os.path.dirname(__file__), '..', 'data', 'aircraft.csv')

def main():
    src = sys.argv[1] if len(sys.argv) > 1 else '/tmp/mictronics/icao24plus.txt'
    
    existing = set()
    with open(AIRCRAFT_CSV) as f:
        r = csv.reader(f); next(r)
        for row in r:
            existing.add(row[0].upper())

    new = []
    with open(src, encoding='latin-1') as f:
        for i, line in enumerate(f):
            if i == 0: continue
            parts = line.strip().split('\t')
            if len(parts) < 2: continue
            h = parts[0].upper()
            if h in existing: continue
            reg = parts[1] if len(parts) > 1 else ''
            icao_type = parts[2] if len(parts) > 2 else ''
            mfr = parts[3].strip() if len(parts) > 3 else ''
            model = parts[4].strip() if len(parts) > 4 else ''
            combined = (mfr + ' ' + model).strip() if mfr else model
            new.append([h, reg, icao_type, '', mfr, combined, '', '', '0', '0', '0'])

    if new:
        with open(AIRCRAFT_CSV, 'a', newline='') as f:
            csv.writer(f).writerows(new)

    print(f'Mictronics: +{len(new)} new aircraft (total: {len(existing) + len(new)})')

if __name__ == '__main__':
    main()
