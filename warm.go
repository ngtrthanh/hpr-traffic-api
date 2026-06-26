package main

import (
	"database/sql"
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var warmDB *sql.DB

// Ship cache: top N in RAM, rest in SQLite
var (
	shipCache     sync.Map // mmsi → *Ship (hot subset)
	shipCacheSize int
	warmShipCount int
	warmAircraftCount int
)

func initWarmStore() error {
	var err error
	warmDB, err = sql.Open("sqlite", "/tmp/hpr-warm.db?_journal=WAL&_sync=NORMAL")
	if err != nil {
		return err
	}
	warmDB.SetMaxOpenConns(4)

	// Create tables
	warmDB.Exec(`CREATE TABLE ships (
		mmsi TEXT PRIMARY KEY, imo TEXT, call_sign TEXT, name TEXT,
		country TEXT, country_code TEXT, gt INTEGER, ship_type INTEGER,
		length_m INTEGER, beam_m INTEGER, class TEXT
	)`)
	warmDB.Exec(`CREATE INDEX idx_ships_imo ON ships(imo) WHERE imo != ''`)
	warmDB.Exec(`CREATE INDEX idx_ships_name ON ships(name)`)
	warmDB.Exec(`CREATE INDEX idx_ships_callsign ON ships(call_sign) WHERE call_sign != ''`)

	warmDB.Exec(`CREATE TABLE aircraft (
		mode_s TEXT PRIMARY KEY, reg TEXT, icao_type TEXT, short_type TEXT,
		manufacturer TEXT, model TEXT, owner TEXT, year TEXT,
		mil INTEGER, pia INTEGER, ladd INTEGER
	)`)
	warmDB.Exec(`CREATE INDEX idx_aircraft_reg ON aircraft(reg) WHERE reg != ''`)

	return nil
}

func loadShipsToWarm(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.Read() // skip header

	tx, _ := warmDB.Begin()
	stmt, _ := tx.Prepare(`INSERT OR REPLACE INTO ships (mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)

	count := 0
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 19 {
			continue
		}
		mmsi := rec[0]
		if mmsi == "" {
			continue
		}
		imo := rec[1]
		callsign := rec[2]
		name := rec[4]
		country := rec[5]
		cc := ituCountryCode(country)
		gt, _ := strconv.Atoi(rec[11])
		st, _ := strconv.Atoi(rec[17])
		ln, _ := strconv.Atoi(rec[18])
		bm := 0
		if len(rec) > 19 {
			bm, _ = strconv.Atoi(rec[19])
		}
		cls := ""
		if len(rec) > 8 {
			cls = rec[8]
		}

		stmt.Exec(mmsi, imo, callsign, name, country, cc, gt, st, ln, bm, cls)
		count++

		// Keep top ships in hot cache (GT > 50000)
		if gt > 50000 {
			shipCache.Store(mmsi, &Ship{
				MMSI: mmsi, IMO: imo, CallSign: callsign, Name: name,
				Country: country, CountryCode: cc,
				GrossTonnage: gt, ShipType: st, LengthM: ln, BeamM: bm, Class: cls,
			})
			shipCacheSize++
		}
	}
	stmt.Close()
	tx.Commit()

	// Also index by IMO for hot cache
	shipCache.Range(func(k, v any) bool {
		s := v.(*Ship)
		if s.IMO != "" {
			shipCache.Store("imo:"+s.IMO, s)
		}
		return true
	})

	log.Printf("[warm] ships: %d in SQLite, %d in hot cache", count, shipCacheSize)
	warmShipCount = count
	return nil
}

func loadAircraftToWarm(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.Read() // skip header

	tx, _ := warmDB.Begin()
	stmt, _ := tx.Prepare(`INSERT OR REPLACE INTO aircraft (mode_s,reg,icao_type,short_type,manufacturer,model,owner,year,mil,pia,ladd) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)

	count := 0
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 11 {
			continue
		}
		modeS := strings.ToUpper(rec[0])
		if modeS == "" {
			continue
		}
		mil, pia, ladd := 0, 0, 0
		if rec[8] == "1" {
			mil = 1
		}
		if rec[9] == "1" {
			pia = 1
		}
		if rec[10] == "1" {
			ladd = 1
		}
		stmt.Exec(modeS, rec[1], rec[2], rec[3], rec[4], rec[5], rec[6], rec[7], mil, pia, ladd)
		count++
	}
	stmt.Close()
	tx.Commit()

	log.Printf("[warm] aircraft: %d in SQLite", count)
	warmAircraftCount = count
	return nil
}

// Lookup ship: hot cache first, then SQLite warm
func warmLookupShip(mmsi string) *Ship {
	// Hot cache
	if v, ok := shipCache.Load(mmsi); ok {
		return v.(*Ship)
	}
	// SQLite
	row := warmDB.QueryRow(`SELECT mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class FROM ships WHERE mmsi=?`, mmsi)
	s := &Ship{}
	if err := row.Scan(&s.MMSI, &s.IMO, &s.CallSign, &s.Name, &s.Country, &s.CountryCode, &s.GrossTonnage, &s.ShipType, &s.LengthM, &s.BeamM, &s.Class); err != nil {
		return nil
	}
	return s
}

// Lookup ship by IMO
func warmLookupShipByIMO(imo string) *Ship {
	if v, ok := shipCache.Load("imo:" + imo); ok {
		return v.(*Ship)
	}
	row := warmDB.QueryRow(`SELECT mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class FROM ships WHERE imo=?`, imo)
	s := &Ship{}
	if err := row.Scan(&s.MMSI, &s.IMO, &s.CallSign, &s.Name, &s.Country, &s.CountryCode, &s.GrossTonnage, &s.ShipType, &s.LengthM, &s.BeamM, &s.Class); err != nil {
		return nil
	}
	return s
}

// Lookup ship by callsign
func warmLookupShipByCallSign(cs string) *Ship {
	row := warmDB.QueryRow(`SELECT mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class FROM ships WHERE call_sign=?`, strings.ToUpper(cs))
	s := &Ship{}
	if err := row.Scan(&s.MMSI, &s.IMO, &s.CallSign, &s.Name, &s.Country, &s.CountryCode, &s.GrossTonnage, &s.ShipType, &s.LengthM, &s.BeamM, &s.Class); err != nil {
		return nil
	}
	return s
}

// Search ships by name/MMSI/callsign/IMO (warm store)
func warmSearchShips(q string, limit int) []*Ship {
	q = strings.ToUpper(q)
	rows, err := warmDB.Query(`SELECT mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class FROM ships WHERE name LIKE ? OR mmsi LIKE ? OR call_sign LIKE ? OR imo LIKE ? ORDER BY gt DESC LIMIT ?`,
		"%"+q+"%", q+"%", q+"%", q+"%", limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*Ship
	for rows.Next() {
		s := &Ship{}
		rows.Scan(&s.MMSI, &s.IMO, &s.CallSign, &s.Name, &s.Country, &s.CountryCode, &s.GrossTonnage, &s.ShipType, &s.LengthM, &s.BeamM, &s.Class)
		result = append(result, s)
	}
	return result
}

// Paginated ship list from warm store
func warmListShips(sortKey string, desc bool, countryFilter, nameFilter string, offset, limit int) (int, []*Ship) {
	where := "WHERE 1=1"
	args := []any{}
	if countryFilter != "" {
		where += " AND country_code=?"
		args = append(args, countryFilter)
	}
	if nameFilter != "" {
		nf := "%" + strings.ToUpper(nameFilter) + "%"
		where += " AND (name LIKE ? OR mmsi LIKE ? OR call_sign LIKE ? OR imo LIKE ?)"
		args = append(args, nf, nameFilter, nameFilter, nameFilter)
	}

	// Count
	var total int
	row := warmDB.QueryRow("SELECT COUNT(*) FROM ships "+where, args...)
	row.Scan(&total)

	// Sort
	orderCol := "gt"
	switch sortKey {
	case "name":
		orderCol = "name"
	case "length":
		orderCol = "length_m"
	case "mmsi":
		orderCol = "mmsi"
	}
	dir := "DESC"
	if !desc {
		dir = "ASC"
	}

	query := "SELECT mmsi,imo,call_sign,name,country,country_code,gt,ship_type,length_m,beam_m,class FROM ships " + where + " ORDER BY " + orderCol + " " + dir + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := warmDB.Query(query, args...)
	if err != nil {
		return total, nil
	}
	defer rows.Close()
	var result []*Ship
	for rows.Next() {
		s := &Ship{}
		rows.Scan(&s.MMSI, &s.IMO, &s.CallSign, &s.Name, &s.Country, &s.CountryCode, &s.GrossTonnage, &s.ShipType, &s.LengthM, &s.BeamM, &s.Class)
		result = append(result, s)
	}
	return total, result
}

// Lookup mode_s by registration
func warmRegToModeS(reg string) string {
	var modeS string
	warmDB.QueryRow(`SELECT mode_s FROM aircraft WHERE reg=?`, strings.ToUpper(reg)).Scan(&modeS)
	return modeS
}

// Lookup aircraft from warm store
func warmLookupAircraft(id string) *Aircraft {
	id = strings.ToUpper(id)
	row := warmDB.QueryRow(`SELECT mode_s,reg,icao_type,short_type,manufacturer,model,owner,year,mil,pia,ladd FROM aircraft WHERE mode_s=? OR reg=?`, id, id)
	var mil, pia, ladd int
	a := &Aircraft{}
	if err := row.Scan(&a.ModeS, &a.Registration, &a.ICAOType, &a.ShortType, &a.Manufacturer, &a.Model, &a.Owner, &a.Year, &mil, &pia, &ladd); err != nil {
		return nil
	}
	a.Mil = mil == 1
	a.PIA = pia == 1
	a.LADD = ladd == 1
	return a
}
