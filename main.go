package main

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// ===================== MIDDLEWARE =====================

// --- Rate Limiter (per-IP, sliding window) ---

type visitor struct {
	count    int
	resetAt  time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{visitors: make(map[string]*visitor), limit: limit, window: window}
	go func() {
		for range time.Tick(window) {
			rl.mu.Lock()
			now := time.Now()
			for k, v := range rl.visitors {
				if now.After(v.resetAt) {
					delete(rl.visitors, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *RateLimiter) Allow(ip string) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	v, ok := rl.visitors[ip]
	if !ok || now.After(v.resetAt) {
		rl.visitors[ip] = &visitor{count: 1, resetAt: now.Add(rl.window)}
		return true, rl.limit - 1
	}
	v.count++
	if v.count > rl.limit {
		return false, 0
	}
	return true, rl.limit - v.count
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	if xri := r.Header.Get("CF-Connecting-IP"); xri != "" {
		return xri
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// --- Middleware chain ---

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			ok, remaining := rl.Allow(ip)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(429)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %s %d %s", clientIP(r), r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// ===================== DATA TYPES =====================

type Aircraft struct {
	ModeS        string `json:"mode_s"`
	Registration string `json:"registration"`
	ICAOType     string `json:"icao_type"`
	ShortType    string `json:"short_type,omitempty"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Owner        string `json:"registered_owner"`
	Year         string `json:"year,omitempty"`
	Mil          bool   `json:"military"`
	PIA          bool   `json:"faa_pia"`
	LADD         bool   `json:"faa_ladd"`
}

type Airline struct {
	Name     string `json:"name"`
	ICAO     string `json:"icao"`
	IATA     string `json:"iata"`
	Country  string `json:"country"`
	Callsign string `json:"callsign"`
}

type Airport struct {
	ICAO      string  `json:"icao_code"`
	IATA      string  `json:"iata_code"`
	Name      string  `json:"name"`
	City      string  `json:"municipality"`
	Country   string  `json:"country_name"`
	Lat       float64 `json:"latitude"`
	Lon       float64 `json:"longitude"`
	Elevation int     `json:"elevation"`
}

type Route struct {
	Callsign     string `json:"callsign"`
	Code         string `json:"code"`
	Number       string `json:"number"`
	AirlineCode  string `json:"airline_code"`
	AirportCodes string `json:"airport_codes"`
}

type FlightRoute struct {
	Callsign     string   `json:"callsign"`
	CallsignICAO string   `json:"callsign_icao"`
	Airline      *Airline `json:"airline"`
	Origin       *Airport `json:"origin"`
	Destination  *Airport `json:"destination"`
}

type CountItem struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type Seaport struct {
	WPIID              string  `json:"wpi_id"`
	Name               string  `json:"name"`
	Country            string  `json:"country"`
	State              string  `json:"state,omitempty"`
	Lat                float64 `json:"latitude"`
	Lon                float64 `json:"longitude"`
	PortSize           string  `json:"port_size"`
	MaxVesselSize      string  `json:"max_vessel_size"`
	ChannelDepth       float64 `json:"channel_depth_m,omitempty"`
	CargoDepth         float64 `json:"cargo_depth_m,omitempty"`
	AnchorageDepth     float64 `json:"anchorage_depth_m,omitempty"`
	OilTerminalDepth   float64 `json:"oil_terminal_depth_m,omitempty"`
	TidalRange         float64 `json:"tidal_range_m,omitempty"`
	EntranceRestriction string `json:"entrance_restriction,omitempty"`
	LOCODE             string  `json:"locode,omitempty"`
	ZoneCode           string  `json:"zone_code,omitempty"`
	VesselCountTotal   int     `json:"vessel_count_total,omitempty"`
	VesselCountContainer int   `json:"vessel_count_container,omitempty"`
	VesselCountDryBulk int     `json:"vessel_count_dry_bulk,omitempty"`
	VesselCountTanker  int     `json:"vessel_count_tanker,omitempty"`
	IndustryTop1       string  `json:"industry_top1,omitempty"`
}

type SeaRoute struct {
	Origin      string  `json:"origin"`
	Destination string  `json:"destination"`
	DistanceNM  float64 `json:"distance_nm"`
	Type        string  `json:"type"` // port or junction
}

type Ship struct {
	MMSI         string `json:"mmsi"`
	CallSign     string `json:"call_sign,omitempty"`
	Name         string `json:"name"`
	Country      string `json:"country,omitempty"`
	GrossTonnage int    `json:"gross_tonnage,omitempty"`
	ShipType     int    `json:"ship_type,omitempty"`
	LengthM      int    `json:"length_m,omitempty"`
	BeamM        int    `json:"beam_m,omitempty"`
	Class        string `json:"class,omitempty"`
}

// ===================== DATA STORES =====================

var (
	aircraft    map[string]Aircraft
	regToModeS  map[string]string
	nNumToModeS map[string]string
	airlines    map[string]Airline
	airports    map[string]Airport
	routes      map[string]Route
	byAirline   map[string][]Route
	byAirport   map[string][]Route
	seaports    []Seaport
	portByLOCODE map[string]*Seaport
	portByWPI    map[string]*Seaport
	portsByCountry map[string][]*Seaport
	portsByZone  map[string][]*Seaport
	seaRoutesByOrigin map[string][]SeaRoute
	shippingLanesJSON []byte
	ships          map[string]*Ship
	shipsByCallSign map[string]*Ship
	startTime   time.Time
)

// ===================== LOADERS =====================

func loadAircraft(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	aircraft = make(map[string]Aircraft, 620000)
	regToModeS = make(map[string]string, 620000)
	nNumToModeS = make(map[string]string, 300000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		ac := Aircraft{
			ModeS: rec[0], Registration: rec[1], ICAOType: rec[2], ShortType: rec[3],
			Manufacturer: rec[4], Model: rec[5], Owner: rec[6], Year: rec[7],
			Mil: rec[8] == "1", PIA: rec[9] == "1", LADD: rec[10] == "1",
		}
		aircraft[rec[0]] = ac
		regToModeS[rec[1]] = rec[0]
		if strings.HasPrefix(rec[1], "N") {
			nNumToModeS[rec[1]] = rec[0]
		}
	}
	return nil
}

func loadAirlines(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	airlines = make(map[string]Airline, 6000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		airlines[rec[1]] = Airline{Name: rec[0], ICAO: rec[1], IATA: rec[2], Country: rec[3], Callsign: rec[4]}
	}
	return nil
}

func loadAirports(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	airports = make(map[string]Airport, 8000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		lat, _ := strconv.ParseFloat(rec[5], 64)
		lon, _ := strconv.ParseFloat(rec[6], 64)
		elev, _ := strconv.Atoi(rec[7])
		airports[rec[0]] = Airport{ICAO: rec[0], IATA: rec[1], Name: rec[2], City: rec[3], Country: rec[4], Lat: lat, Lon: lon, Elevation: elev}
	}
	return nil
}

func loadRoutes(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	routes = make(map[string]Route, 500000)
	byAirline = make(map[string][]Route)
	byAirport = make(map[string][]Route)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		rt := Route{rec[0], rec[1], rec[2], rec[3], rec[4]}
		routes[rec[0]] = rt
		byAirline[rt.AirlineCode] = append(byAirline[rt.AirlineCode], rt)
		for _, ap := range strings.Split(rt.AirportCodes, "-") {
			byAirport[ap] = append(byAirport[ap], rt)
		}
	}
	return nil
}

// ===================== HELPERS =====================

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Asin(math.Sqrt(a))
}

func loadSeaports(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read() // skip header
	portByLOCODE = make(map[string]*Seaport, 4000)
	portByWPI = make(map[string]*Seaport, 4000)
	portsByCountry = make(map[string][]*Seaport)
	portsByZone = make(map[string][]*Seaport)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		lat, _ := strconv.ParseFloat(rec[4], 64)
		lon, _ := strconv.ParseFloat(rec[5], 64)
		chD, _ := strconv.ParseFloat(rec[8], 64)
		caD, _ := strconv.ParseFloat(rec[9], 64)
		anD, _ := strconv.ParseFloat(rec[10], 64)
		oiD, _ := strconv.ParseFloat(rec[11], 64)
		tid, _ := strconv.ParseFloat(rec[12], 64)
		vt, _ := strconv.Atoi(rec[16])
		vc, _ := strconv.Atoi(rec[17])
		vb, _ := strconv.Atoi(rec[18])
		vk, _ := strconv.Atoi(rec[19])
		sp := Seaport{
			WPIID: rec[0], Name: rec[1], Country: rec[2], State: rec[3],
			Lat: lat, Lon: lon,
			PortSize: rec[6], MaxVesselSize: rec[7],
			ChannelDepth: chD, CargoDepth: caD, AnchorageDepth: anD, OilTerminalDepth: oiD,
			TidalRange: tid, EntranceRestriction: rec[13],
			LOCODE: rec[14], ZoneCode: rec[15],
			VesselCountTotal: vt, VesselCountContainer: vc, VesselCountDryBulk: vb, VesselCountTanker: vk,
			IndustryTop1: rec[20],
		}
		seaports = append(seaports, sp)
		ptr := &seaports[len(seaports)-1]
		if sp.LOCODE != "" {
			portByLOCODE[sp.LOCODE] = ptr
		}
		portByWPI[sp.WPIID] = ptr
		portsByCountry[strings.ToUpper(sp.Country)] = append(portsByCountry[strings.ToUpper(sp.Country)], ptr)
		if sp.ZoneCode != "" {
			portsByZone[sp.ZoneCode] = append(portsByZone[sp.ZoneCode], ptr)
		}
	}
	return nil
}

func loadSeaRoutes(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	seaRoutesByOrigin = make(map[string][]SeaRoute, 2000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		dist, _ := strconv.ParseFloat(rec[2], 64)
		sr := SeaRoute{Origin: rec[0], Destination: rec[1], DistanceNM: dist, Type: rec[3]}
		key := strings.ToUpper(rec[0])
		seaRoutesByOrigin[key] = append(seaRoutesByOrigin[key], sr)
	}
	return nil
}

func loadShippingLanes(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	shippingLanesJSON = data
	return nil
}

func loadShips(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	ships = make(map[string]*Ship, 750000)
	shipsByCallSign = make(map[string]*Ship, 700000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		gt, _ := strconv.Atoi(rec[10])
		st, _ := strconv.Atoi(rec[16])
		ln, _ := strconv.Atoi(rec[17])
		bm, _ := strconv.Atoi(rec[18])
		s := &Ship{
			MMSI: rec[0], CallSign: rec[1], Name: rec[3], Country: rec[4],
			GrossTonnage: gt, ShipType: st, LengthM: ln, BeamM: bm, Class: rec[7],
		}
		ships[rec[0]] = s
		if rec[1] != "" {
			shipsByCallSign[strings.ToUpper(rec[1])] = s
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	fmt.Fprintf(w, `{"response":"%s"}`, msg)
}

func qInt(r *http.Request, key string, def, max int) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func topN(m map[string][]Route, n int) []CountItem {
	items := make([]CountItem, 0, len(m))
	for k, v := range m {
		items = append(items, CountItem{k, len(v)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func paginate(rts []Route, limit, offset int) []Route {
	if offset >= len(rts) {
		return []Route{}
	}
	end := offset + limit
	if end > len(rts) {
		end = len(rts)
	}
	return rts[offset:end]
}

// ===================== MAIN =====================

func main() {
	for _, l := range []struct {
		name string
		fn   func(string) error
		path string
	}{
		{"aircraft", loadAircraft, "aircraft.csv"},
		{"airlines", loadAirlines, "airlines.csv"},
		{"airports", loadAirports, "airports.csv"},
		{"routes", loadRoutes, "routes.csv"},
		{"seaports", loadSeaports, "seaports.csv"},
		{"sea_routes", loadSeaRoutes, "sea_distances.csv"},
		{"shipping_lanes", loadShippingLanes, "shipping_lanes.geojson"},
		{"ships", loadShips, "ships.csv"},
	} {
		if err := l.fn(l.path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load %s: %v\n", l.name, err)
			os.Exit(1)
		}
	}
	startTime = time.Now()
	fmt.Printf("Loaded: %d aircraft, %d airlines, %d airports, %d routes, %d seaports, %d sea-route origins, %d ships\n",
		len(aircraft), len(airlines), len(airports), len(routes), len(seaports), len(seaRoutesByOrigin), len(ships))

	mux := http.NewServeMux()

	// === adsbdb-compatible v0 ===

	mux.HandleFunc("/v0/aircraft/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.ToUpper(r.URL.Path[len("/v0/aircraft/"):])
		ac, ok := aircraft[key]
		if !ok {
			if ms, found := regToModeS[key]; found {
				ac, ok = aircraft[ms]
			}
		}
		if ok {
			writeJSON(w, map[string]any{"response": map[string]any{"aircraft": ac}})
		} else {
			notFound(w, "unknown aircraft")
		}
	})

	mux.HandleFunc("/v0/callsign/", func(w http.ResponseWriter, r *http.Request) {
		cs := strings.ToUpper(r.URL.Path[len("/v0/callsign/"):])
		rt, ok := routes[cs]
		if !ok {
			notFound(w, "unknown callsign")
			return
		}
		fr := FlightRoute{Callsign: cs, CallsignICAO: cs}
		if al, ok := airlines[rt.AirlineCode]; ok {
			fr.Airline = &al
		}
		parts := strings.Split(rt.AirportCodes, "-")
		if len(parts) >= 2 {
			if ap, ok := airports[parts[0]]; ok {
				fr.Origin = &ap
			}
			if ap, ok := airports[parts[1]]; ok {
				fr.Destination = &ap
			}
		}
		writeJSON(w, map[string]any{"response": map[string]any{"flightroute": fr}})
	})

	mux.HandleFunc("/v0/airline/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v0/airline/"):])
		if al, ok := airlines[code]; ok {
			writeJSON(w, map[string]any{"response": []Airline{al}})
		} else {
			notFound(w, "unknown airline")
		}
	})

	mux.HandleFunc("/v0/n-number/", func(w http.ResponseWriter, r *http.Request) {
		n := strings.ToUpper(r.URL.Path[len("/v0/n-number/"):])
		if !strings.HasPrefix(n, "N") {
			n = "N" + n
		}
		if ms, ok := nNumToModeS[n]; ok {
			writeJSON(w, map[string]any{"response": ms})
		} else {
			notFound(w, "unknown n-number")
		}
	})

	mux.HandleFunc("/v0/mode-s/", func(w http.ResponseWriter, r *http.Request) {
		hex := strings.ToUpper(r.URL.Path[len("/v0/mode-s/"):])
		if ac, ok := aircraft[hex]; ok {
			writeJSON(w, map[string]any{"response": ac.Registration})
		} else {
			notFound(w, "unknown mode-s")
		}
	})

	mux.HandleFunc("/v0/online", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"response": map[string]any{
			"uptime":      int(time.Since(startTime).Seconds()),
			"api_version": "1.0.0",
		}})
	})

	// === Extended v1 ===

	mux.HandleFunc("/v1/routes/", func(w http.ResponseWriter, r *http.Request) {
		cs := strings.ToUpper(r.URL.Path[len("/v1/routes/"):])
		if rt, ok := routes[cs]; ok {
			writeJSON(w, rt)
		} else {
			notFound(w, "Route not found")
		}
	})

	mux.HandleFunc("/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"aviation": map[string]any{
				"aircraft": len(aircraft),
				"routes":   len(routes),
				"airlines": len(byAirline),
				"airports": len(byAirport),
			},
			"maritime": map[string]any{
				"ships":            len(ships),
				"seaports":         len(seaports),
				"sea_route_origins": len(seaRoutesByOrigin),
				"ports_with_locode": len(portByLOCODE),
				"countries":        len(portsByCountry),
				"zones":            len(portsByZone),
			},
			"top_airlines": topN(byAirline, 20),
			"top_airports": topN(byAirport, 20),
		})
	})

	mux.HandleFunc("/v1/airlines/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/airlines/"):])
		rts := byAirline[code]
		if len(rts) == 0 {
			notFound(w, "Airline not found")
			return
		}
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(rts))
		writeJSON(w, map[string]any{
			"airline": code, "total_routes": len(rts),
			"limit": limit, "offset": offset,
			"routes": paginate(rts, limit, offset),
		})
	})

	mux.HandleFunc("/v1/airports/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/airports/"):])
		rts := byAirport[code]
		if len(rts) == 0 {
			notFound(w, "Airport not found")
			return
		}
		connected := make(map[string]int)
		for _, rt := range rts {
			for _, p := range strings.Split(rt.AirportCodes, "-") {
				if p != code {
					connected[p]++
				}
			}
		}
		conns := make([]CountItem, 0, len(connected))
		for k, v := range connected {
			conns = append(conns, CountItem{k, v})
		}
		sort.Slice(conns, func(i, j int) bool { return conns[i].Count > conns[j].Count })
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(rts))
		writeJSON(w, map[string]any{
			"airport": code, "total_routes": len(rts),
			"connected_airports": len(connected), "top_connections": conns,
			"limit": limit, "offset": offset,
			"routes": paginate(rts, limit, offset),
		})
	})

	// === Maritime ports v1 ===

	mux.HandleFunc("/v1/ports/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total_ports":    len(seaports),
			"with_locode":   len(portByLOCODE),
			"total_countries": len(portsByCountry),
			"total_zones":   len(portsByZone),
		})
	})

	mux.HandleFunc("/v1/ports/nearby", func(w http.ResponseWriter, r *http.Request) {
		lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
		lon, _ := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
		radius := qInt(r, "radius_km", 50, 500)
		limit := qInt(r, "limit", 20, 100)
		type portDist struct {
			Port     *Seaport `json:"port"`
			Distance float64  `json:"distance_km"`
		}
		var results []portDist
		for i := range seaports {
			d := haversineKm(lat, lon, seaports[i].Lat, seaports[i].Lon)
			if d <= float64(radius) {
				results = append(results, portDist{&seaports[i], math.Round(d*10) / 10})
			}
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Distance < results[j].Distance })
		if len(results) > limit {
			results = results[:limit]
		}
		writeJSON(w, map[string]any{"lat": lat, "lon": lon, "radius_km": radius, "count": len(results), "ports": results})
	})

	mux.HandleFunc("/v1/ports/country/", func(w http.ResponseWriter, r *http.Request) {
		country := strings.ToUpper(r.URL.Path[len("/v1/ports/country/"):])
		pts := portsByCountry[country]
		if len(pts) == 0 {
			notFound(w, "Country not found")
			return
		}
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(pts))
		end := offset + limit
		if end > len(pts) {
			end = len(pts)
		}
		page := pts[offset:end]
		writeJSON(w, map[string]any{"country": country, "total_ports": len(pts), "limit": limit, "offset": offset, "ports": page})
	})

	mux.HandleFunc("/v1/ports/zone/", func(w http.ResponseWriter, r *http.Request) {
		zone := r.URL.Path[len("/v1/ports/zone/"):]
		pts := portsByZone[zone]
		if len(pts) == 0 {
			notFound(w, "Zone not found")
			return
		}
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(pts))
		end := offset + limit
		if end > len(pts) {
			end = len(pts)
		}
		page := pts[offset:end]
		writeJSON(w, map[string]any{"zone": zone, "total_ports": len(pts), "limit": limit, "offset": offset, "ports": page})
	})

	mux.HandleFunc("/v1/ports/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.ToUpper(r.URL.Path[len("/v1/ports/"):])
		if p, ok := portByLOCODE[id]; ok {
			writeJSON(w, p)
			return
		}
		if p, ok := portByWPI[id]; ok {
			writeJSON(w, p)
			return
		}
		notFound(w, "Port not found")
	})

	// === Sea routes v1 ===

	mux.HandleFunc("/v1/sea-routes/from/", func(w http.ResponseWriter, r *http.Request) {
		origin := r.URL.Path[len("/v1/sea-routes/from/"):]
		key := strings.ToUpper(origin)
		rts := seaRoutesByOrigin[key]
		if len(rts) == 0 {
			// Try case-sensitive match
			for k, v := range seaRoutesByOrigin {
				if strings.EqualFold(k, origin) || strings.Contains(strings.ToUpper(k), key) {
					rts = v
					break
				}
			}
		}
		if len(rts) == 0 {
			notFound(w, "Origin port not found")
			return
		}
		// Split into port destinations and junctions
		var ports, junctions []SeaRoute
		for _, rt := range rts {
			if rt.Type == "junction" {
				junctions = append(junctions, rt)
			} else {
				ports = append(ports, rt)
			}
		}
		sort.Slice(ports, func(i, j int) bool { return ports[i].DistanceNM < ports[j].DistanceNM })
		sort.Slice(junctions, func(i, j int) bool { return junctions[i].DistanceNM < junctions[j].DistanceNM })
		writeJSON(w, map[string]any{
			"origin": rts[0].Origin, "destinations": ports, "junctions": junctions,
		})
	})

	mux.HandleFunc("/v1/sea-routes/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToUpper(r.URL.Query().Get("q"))
		if q == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"q parameter required"}`))
			return
		}
		var matches []string
		for k := range seaRoutesByOrigin {
			if strings.Contains(k, q) {
				matches = append(matches, seaRoutesByOrigin[k][0].Origin)
			}
		}
		sort.Strings(matches)
		if len(matches) > 50 {
			matches = matches[:50]
		}
		writeJSON(w, map[string]any{"query": q, "count": len(matches), "ports": matches})
	})

	mux.HandleFunc("/v1/shipping-lanes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(shippingLanesJSON)
	})

	// === Ships v1 ===

	mux.HandleFunc("/v1/ships/callsign/", func(w http.ResponseWriter, r *http.Request) {
		cs := strings.ToUpper(r.URL.Path[len("/v1/ships/callsign/"):])
		if s, ok := shipsByCallSign[cs]; ok {
			writeJSON(w, s)
		} else {
			notFound(w, "Ship not found")
		}
	})

	mux.HandleFunc("/v1/ships/", func(w http.ResponseWriter, r *http.Request) {
		mmsi := r.URL.Path[len("/v1/ships/"):]
		if s, ok := ships[mmsi]; ok {
			writeJSON(w, s)
		} else {
			notFound(w, "Ship not found")
		}
	})

	// === MCP v1 ===

	mcpManifest, _ := json.Marshal(map[string]any{
		"name": "hpr-traffic-api", "version": "1.0.0",
		"description": "Aviation and maritime traffic data API",
		"tools": []map[string]any{
			{"name": "lookup_flight_route", "description": "Get origin/destination for a flight callsign", "parameters": map[string]string{"callsign": "string"}},
			{"name": "lookup_aircraft", "description": "Get aircraft info by Mode-S hex or registration", "parameters": map[string]string{"id": "string"}},
			{"name": "lookup_ship", "description": "Get ship details by MMSI or callsign", "parameters": map[string]string{"id": "string"}},
			{"name": "lookup_port", "description": "Get seaport details by LOCODE or WPI ID", "parameters": map[string]string{"id": "string"}},
			{"name": "sea_distance", "description": "Get distances from a port to all connected destinations", "parameters": map[string]string{"port": "string"}},
			{"name": "nearby_ports", "description": "Find seaports within radius of coordinates", "parameters": map[string]string{"lat": "number", "lon": "number", "radius_km": "number"}},
			{"name": "search_sea_routes", "description": "Search for ports in the sea distance database", "parameters": map[string]string{"query": "string"}},
		},
	})

	mux.HandleFunc("/v1/mcp/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			Tool   string         `json:"tool"`
			Params map[string]any `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid JSON"}`))
			return
		}
		ps := func(k string) string {
			if v, ok := req.Params[k]; ok {
				return fmt.Sprintf("%v", v)
			}
			return ""
		}
		pf := func(k string) float64 {
			if v, ok := req.Params[k]; ok {
				if f, ok := v.(float64); ok {
					return f
				}
			}
			return 0
		}
		switch req.Tool {
		case "lookup_flight_route":
			cs := strings.ToUpper(ps("callsign"))
			if rt, ok := routes[cs]; ok {
				writeJSON(w, rt)
			} else {
				notFound(w, "Route not found")
			}
		case "lookup_aircraft":
			id := strings.ToUpper(ps("id"))
			if a, ok := aircraft[id]; ok {
				writeJSON(w, a)
			} else if hex, ok := regToModeS[id]; ok {
				writeJSON(w, aircraft[hex])
			} else {
				notFound(w, "Aircraft not found")
			}
		case "lookup_ship":
			id := strings.ToUpper(ps("id"))
			if s, ok := ships[id]; ok {
				writeJSON(w, s)
			} else if s, ok := shipsByCallSign[id]; ok {
				writeJSON(w, s)
			} else {
				notFound(w, "Ship not found")
			}
		case "lookup_port":
			id := strings.ToUpper(ps("id"))
			if p, ok := portByLOCODE[id]; ok {
				writeJSON(w, p)
			} else if p, ok := portByWPI[id]; ok {
				writeJSON(w, p)
			} else {
				notFound(w, "Port not found")
			}
		case "sea_distance":
			origin := ps("port")
			key := strings.ToUpper(origin)
			rts := seaRoutesByOrigin[key]
			if len(rts) == 0 {
				for k, v := range seaRoutesByOrigin {
					if strings.EqualFold(k, origin) || strings.Contains(strings.ToUpper(k), key) {
						rts = v
						break
					}
				}
			}
			if len(rts) == 0 {
				notFound(w, "Origin port not found")
				return
			}
			var ports, junctions []SeaRoute
			for _, rt := range rts {
				if rt.Type == "junction" {
					junctions = append(junctions, rt)
				} else {
					ports = append(ports, rt)
				}
			}
			writeJSON(w, map[string]any{"origin": rts[0].Origin, "destinations": ports, "junctions": junctions})
		case "nearby_ports":
			lat, lon, radius := pf("lat"), pf("lon"), pf("radius_km")
			if radius == 0 {
				radius = 50
			}
			type portDist struct {
				Port     *Seaport `json:"port"`
				Distance float64  `json:"distance_km"`
			}
			var results []portDist
			for i := range seaports {
				d := haversineKm(lat, lon, seaports[i].Lat, seaports[i].Lon)
				if d <= radius {
					results = append(results, portDist{&seaports[i], math.Round(d*10) / 10})
				}
			}
			sort.Slice(results, func(i, j int) bool { return results[i].Distance < results[j].Distance })
			if len(results) > 20 {
				results = results[:20]
			}
			writeJSON(w, map[string]any{"lat": lat, "lon": lon, "radius_km": radius, "count": len(results), "ports": results})
		case "search_sea_routes":
			q := strings.ToUpper(ps("query"))
			var matches []string
			for k := range seaRoutesByOrigin {
				if strings.Contains(strings.ToUpper(k), q) {
					matches = append(matches, k)
				}
			}
			sort.Strings(matches)
			if len(matches) > 50 {
				matches = matches[:50]
			}
			writeJSON(w, map[string]any{"query": ps("query"), "count": len(matches), "ports": matches})
		default:
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"unknown tool"}`))
		}
	})

	mux.HandleFunc("/v1/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mcpManifest)
	})

	// === Batch v1 ===

	mux.HandleFunc("/v1/batch/routes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			Callsigns []string `json:"callsigns"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid JSON"}`))
			return
		}
		if len(req.Callsigns) > 100 {
			req.Callsigns = req.Callsigns[:100]
		}
		results := make([]any, len(req.Callsigns))
		for i, cs := range req.Callsigns {
			if rt, ok := routes[strings.ToUpper(cs)]; ok {
				results[i] = rt
			}
		}
		writeJSON(w, results)
	})

	mux.HandleFunc("/v1/batch/ships", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			MMSIs []string `json:"mmsis"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid JSON"}`))
			return
		}
		if len(req.MMSIs) > 100 {
			req.MMSIs = req.MMSIs[:100]
		}
		results := make([]any, len(req.MMSIs))
		for i, m := range req.MMSIs {
			if s, ok := ships[m]; ok {
				results[i] = s
			}
		}
		writeJSON(w, results)
	})

	// === Ports GeoJSON (for map demo) ===
	mux.HandleFunc("/v1/ports/geojson", func(w http.ResponseWriter, r *http.Request) {
		type feat struct {
			Type string         `json:"type"`
			Geom map[string]any `json:"geometry"`
			Prop map[string]any `json:"properties"`
		}
		features := make([]feat, 0, len(seaports))
		for i := range seaports {
			p := &seaports[i]
			features = append(features, feat{
				Type: "Feature",
				Geom: map[string]any{"type": "Point", "coordinates": []float64{p.Lon, p.Lat}},
				Prop: map[string]any{"name": p.Name, "country": p.Country, "port_size": p.PortSize, "locode": p.LOCODE, "zone_code": p.ZoneCode, "wpi_id": p.WPIID, "max_vessel_size": p.MaxVesselSize, "channel_depth_m": p.ChannelDepth, "cargo_depth_m": p.CargoDepth},
			})
		}
		writeJSON(w, map[string]any{"type": "FeatureCollection", "features": features})
	})

	// === Static demo app ===
	sub, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// === Apply middleware: logging → CORS → rate limit → mux ===
	rl := NewRateLimiter(600, time.Minute) // 600 req/min per IP
	handler := chain(mux, requestLog, cors, rateLimit(rl))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	fmt.Printf("Listening on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
