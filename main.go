package main

import (
	"container/heap"
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version set via -ldflags "-X main.Version=vX.Y.Z"
var Version = "dev"

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
	ICAO        string  `json:"icao_code"`
	IATA        string  `json:"iata_code"`
	Name        string  `json:"name"`
	City        string  `json:"municipality,omitempty"`
	Country     string  `json:"country_code"`
	CountryCode string  `json:"-"`
	Lat         float64 `json:"latitude"`
	Lon         float64 `json:"longitude"`
	Elevation   int     `json:"elevation,omitempty"`
	Type        string  `json:"type,omitempty"`
	Scheduled   bool    `json:"scheduled_service"`
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
	LOCODE        string  `json:"locode"`
	Name          string  `json:"name"`
	CountryCode   string  `json:"country_code"`
	Lat           float64 `json:"latitude"`
	Lon           float64 `json:"longitude"`
	PortSize      string  `json:"port_size,omitempty"`
	MaxVesselSize string  `json:"max_vessel_size,omitempty"`
	ChannelDepth  float64 `json:"channel_depth_m,omitempty"`
	CargoDepth    float64 `json:"cargo_depth_m,omitempty"`
	TEUThousands  int     `json:"teu_thousands,omitempty"`
	ZoneCode      string  `json:"zone_code,omitempty"`
	WPIID         string  `json:"wpi_id,omitempty"`
	Function      string  `json:"function,omitempty"`
	Status        string  `json:"status,omitempty"`
	Active        bool    `json:"active"`
}

type SeaRoute struct {
	Origin      string  `json:"origin"`
	Destination string  `json:"destination"`
	DistanceNM  float64 `json:"distance_nm"`
	Type        string  `json:"type"` // port or junction
}

type Ship struct {
	MMSI         string `json:"mmsi"`
	IMO          string `json:"imo,omitempty"`
	CallSign     string `json:"call_sign,omitempty"`
	Name         string `json:"name"`
	Country      string `json:"country,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
	GrossTonnage int    `json:"gross_tonnage,omitempty"`
	ShipType     int    `json:"ship_type,omitempty"`
	LengthM      int    `json:"length_m,omitempty"`
	BeamM        int    `json:"beam_m,omitempty"`
	Class        string `json:"class,omitempty"`
}

type ShippingCompany struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	CountryCode string `json:"country_code"`
	Sector      string `json:"sector"`
	Parent      string `json:"parent,omitempty"`
	FleetSize   int    `json:"fleet_size"`
	TEUCapacity int    `json:"teu_capacity,omitempty"`
	Website     string `json:"website,omitempty"`
	NamePrefix  string `json:"-"`
	Active      bool   `json:"active"`
	IMOCompany  string `json:"imo_company,omitempty"`
}

type NotableShip struct {
	IMO      string `json:"imo"`
	MMSI     string `json:"mmsi,omitempty"`
	Name     string `json:"name"`
	Flag     string `json:"flag,omitempty"`
	ShipType string `json:"ship_type,omitempty"`
	DWT      int    `json:"dwt,omitempty"`
	GT       int    `json:"gross_tonnage,omitempty"`
	TEU      int    `json:"teu,omitempty"`
	LengthM  int    `json:"length_m,omitempty"`
	BeamM    int    `json:"beam_m,omitempty"`
	YearBuilt int   `json:"year_built,omitempty"`
	Builder  string `json:"builder,omitempty"`
	Operator string `json:"operator,omitempty"`
	Sector   string `json:"sector"`
	Status   string `json:"status"`
	Photo1   string `json:"photo1,omitempty"`
	Photo2   string `json:"photo2,omitempty"`
}

// ===================== DATA STORES =====================

type SeaDistancePort struct {
	Lat float64
	Lon float64
}

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
	seaDistancePorts  map[string]SeaDistancePort
	shippingLanesJSON []byte
	combinedLanesJSON []byte
	ships          map[string]*Ship
	shipsList      []*Ship
	shipsByIMO     map[string]*Ship
	shipsByCallSign map[string]*Ship
	companies       []*ShippingCompany
	companyByCode   map[string]*ShippingCompany
	companyPrefixes []struct{ prefix, code string }
	notableShips    []*NotableShip
	notableByMMSI   map[string]*NotableShip
	notableByName   map[string]*NotableShip
	notableByIMO    map[string]*NotableShip
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
	r.FieldsPerRecord = -1
	r.Read()
	airports = make(map[string]Airport, 10000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 8 {
			continue
		}
		lat, _ := strconv.ParseFloat(rec[5], 64)
		lon, _ := strconv.ParseFloat(rec[6], 64)
		elev, _ := strconv.Atoi(rec[7])
		cc := rec[4]
		if cc == "" {
			cc = icaoCountryCode(rec[0])
		}
		var aptType string
		var scheduled bool
		if len(rec) > 8 {
			aptType = rec[8]
		}
		if len(rec) > 9 {
			scheduled = rec[9] == "yes"
		}
		airports[rec[0]] = Airport{ICAO: rec[0], IATA: rec[1], Name: rec[2], City: rec[3], Country: cc, CountryCode: cc, Lat: lat, Lon: lon, Elevation: elev, Type: aptType, Scheduled: scheduled}
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
	r.FieldsPerRecord = -1
	r.Read() // skip header: locode,name,country_code,latitude,longitude,port_size,max_vessel_size,channel_depth_m,cargo_depth_m,teu_thousands,zone_code,wpi_id,function,status,active
	portByLOCODE = make(map[string]*Seaport, 18000)
	portByWPI = make(map[string]*Seaport, 4000)
	portsByCountry = make(map[string][]*Seaport)
	portsByZone = make(map[string][]*Seaport)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 15 {
			continue
		}
		lat, _ := strconv.ParseFloat(rec[3], 64)
		lon, _ := strconv.ParseFloat(rec[4], 64)
		chD, _ := strconv.ParseFloat(rec[7], 64)
		caD, _ := strconv.ParseFloat(rec[8], 64)
		teu, _ := strconv.Atoi(rec[9])
		sp := Seaport{
			LOCODE: rec[0], Name: rec[1], CountryCode: rec[2],
			Lat: lat, Lon: lon,
			PortSize: rec[5], MaxVesselSize: rec[6],
			ChannelDepth: chD, CargoDepth: caD,
			TEUThousands: teu, ZoneCode: rec[10], WPIID: rec[11],
			Function: rec[12], Status: rec[13], Active: rec[14] == "1",
		}
		seaports = append(seaports, sp)
		ptr := &seaports[len(seaports)-1]
		if sp.LOCODE != "" {
			portByLOCODE[sp.LOCODE] = ptr
		}
		if sp.WPIID != "" {
			portByWPI[sp.WPIID] = ptr
		}
		portsByCountry[strings.ToUpper(sp.CountryCode)] = append(portsByCountry[strings.ToUpper(sp.CountryCode)], ptr)
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

func loadSeaDistancePorts(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	seaDistancePorts = make(map[string]SeaDistancePort, 2000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		lat, _ := strconv.ParseFloat(rec[1], 64)
		lon, _ := strconv.ParseFloat(rec[2], 64)
		seaDistancePorts[strings.ToUpper(rec[0])] = SeaDistancePort{lat, lon}
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

// ===================== MARNET SEA ROUTING GRAPH =====================

// Node index type for the graph
type nodeID int32

// Graph edge with distance (nm) and intermediate coordinates for rendering
type graphEdge struct {
	to     nodeID
	dist   float64
	pass   string       // canal/strait name if this edge is a named passage
	coords [][2]float64 // full polyline [lon,lat] pairs including endpoints
}

var (
	marnetNodes [][2]float64  // [lon,lat] per node
	marnetAdj   [][]graphEdge // adjacency list
	marnetGrid  map[[2]int][]nodeID // spatial grid for nearest-node lookup (1-degree cells)
	marnetJSON  []byte // raw GeoJSON for serving as lanes
)

// quantize coord to grid cell
func gridKey(lon, lat float64) [2]int {
	return [2]int{int(math.Floor(lon)), int(math.Floor(lat))}
}

func loadMarnet(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var gj struct {
		Features []struct {
			Geometry struct {
				Type        string      `json:"type"`
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &gj); err != nil {
		return err
	}

	// Build tiered GeoJSON with distance-based tier classification
	type tieredFeat struct {
		Type string `json:"type"`
		Geom struct {
			Type   string      `json:"type"`
			Coords [][]float64 `json:"coordinates"`
		} `json:"geometry"`
		Props map[string]any `json:"properties"`
	}
	tieredFeatures := make([]tieredFeat, 0, len(gj.Features))

	// Build node index (deduplicate by rounded coords)
	nodeMap := make(map[[2]int32]nodeID)
	getNode := func(lon, lat float64) nodeID {
		// Round to ~11m precision
		key := [2]int32{int32(math.Round(lon * 1e4)), int32(math.Round(lat * 1e4))}
		if id, ok := nodeMap[key]; ok {
			return id
		}
		id := nodeID(len(marnetNodes))
		nodeMap[key] = id
		marnetNodes = append(marnetNodes, [2]float64{lon, lat})
		marnetAdj = append(marnetAdj, nil)
		return id
	}

	for _, feat := range gj.Features {
		coords := feat.Geometry.Coordinates
		if len(coords) < 2 {
			continue
		}
		a := getNode(coords[0][0], coords[0][1])
		b := getNode(coords[len(coords)-1][0], coords[len(coords)-1][1])
		// Compute edge length in nm (sum of segments)
		dist := 0.0
		for i := 1; i < len(coords); i++ {
			dist += haversineNM(coords[i-1][1], coords[i-1][0], coords[i][1], coords[i][0])
		}
		// Tier: 1=major ocean(>=200nm), 2=regional(50-200), 3=coastal(20-50), 4=local(<20)
		tier := 4
		if dist >= 200 {
			tier = 1
		} else if dist >= 50 {
			tier = 2
		} else if dist >= 20 {
			tier = 3
		}
		// Named passages always tier 1
		var passName string
		if feat.Properties != nil {
			if v, ok := feat.Properties["pass"]; ok && v != nil {
				tier = 1
				passName, _ = v.(string)
			}
		}
		var tf tieredFeat
		tf.Type = "Feature"
		tf.Geom.Type = "LineString"
		tf.Geom.Coords = coords
		tf.Props = map[string]any{"tier": tier}
		tieredFeatures = append(tieredFeatures, tf)

		// Store polyline coords
		poly := make([][2]float64, len(coords))
		for i, c := range coords {
			poly[i] = [2]float64{c[0], c[1]}
		}
		polyRev := make([][2]float64, len(coords))
		for i, c := range poly {
			polyRev[len(poly)-1-i] = c
		}
		marnetAdj[a] = append(marnetAdj[a], graphEdge{to: b, dist: dist, pass: passName, coords: poly})
		marnetAdj[b] = append(marnetAdj[b], graphEdge{to: a, dist: dist, pass: passName, coords: polyRev})
	}

	// Serialize tiered GeoJSON — only marnet local detail (tier 4, <20nm edges)
	var localFeatures []tieredFeat
	for _, f := range tieredFeatures {
		if f.Props["tier"] == 4 {
			localFeatures = append(localFeatures, f)
		}
	}
	marnetJSON, _ = json.Marshal(map[string]any{"type": "FeatureCollection", "features": localFeatures})

	// Build spatial grid
	marnetGrid = make(map[[2]int][]nodeID, len(marnetNodes)/4)
	for i, nd := range marnetNodes {
		k := gridKey(nd[0], nd[1])
		marnetGrid[k] = append(marnetGrid[k], nodeID(i))
	}

	fmt.Printf("  marnet: %d nodes, %d edges\n", len(marnetNodes), len(gj.Features))
	return nil
}

func haversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	const deg2rad = math.Pi / 180
	dLat := (lat2 - lat1) * deg2rad
	dLon := (lon2 - lon1) * deg2rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*deg2rad)*math.Cos(lat2*deg2rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * 3440.065 * math.Asin(math.Sqrt(a)) // 3440.065 nm = earth radius in nm
}

// greatCircleArc returns a series of [lon,lat] points along the great circle between two points.
// Uses SLERP (spherical linear interpolation). Number of segments scales with distance.
func greatCircleArc(lat1, lon1, lat2, lon2 float64) [][]float64 {
	const deg2rad = math.Pi / 180
	const rad2deg = 180 / math.Pi
	φ1, λ1 := lat1*deg2rad, lon1*deg2rad
	φ2, λ2 := lat2*deg2rad, lon2*deg2rad

	// Angular distance
	d := math.Acos(math.Sin(φ1)*math.Sin(φ2) + math.Cos(φ1)*math.Cos(φ2)*math.Cos(λ2-λ1))
	if d < 1e-10 {
		return [][]float64{{lon1, lat1}, {lon2, lat2}}
	}

	// Segments: 1 per ~200nm, min 2 max 64
	nm := d * 3440.065
	n := int(nm/200) + 1
	if n < 2 {
		n = 2
	}
	if n > 64 {
		n = 64
	}

	pts := make([][]float64, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		a := math.Sin((1-f)*d) / math.Sin(d)
		b := math.Sin(f*d) / math.Sin(d)
		x := a*math.Cos(φ1)*math.Cos(λ1) + b*math.Cos(φ2)*math.Cos(λ2)
		y := a*math.Cos(φ1)*math.Sin(λ1) + b*math.Cos(φ2)*math.Sin(λ2)
		z := a*math.Sin(φ1) + b*math.Sin(φ2)
		lat := math.Atan2(z, math.Sqrt(x*x+y*y)) * rad2deg
		lon := math.Atan2(y, x) * rad2deg
		// Unwrap longitude to avoid anti-meridian jumps
		if i > 0 {
			for lon-pts[i-1][0] > 180 {
				lon -= 360
			}
			for lon-pts[i-1][0] < -180 {
				lon += 360
			}
		}
		pts[i] = []float64{lon, lat}
	}
	return pts
}

// Find nearest graph node to given lon,lat within search radius
// findAirRoute uses Dijkstra on the flight route network to find shortest multi-hop path.
func findAirRoute(fromICAO, toICAO string) []string {
	// BFS/Dijkstra with haversine distance weights
	type airNode struct {
		icao string
		dist float64
	}
	dist := make(map[string]float64)
	prev := make(map[string]string)
	dist[fromICAO] = 0

	// Priority queue using sorted slice (flight network is sparse enough)
	h := &airPQ{{icao: fromICAO, dist: 0}}
	heap.Init(h)

	for h.Len() > 0 {
		cur := heap.Pop(h).(*airPQItem)
		if cur.icao == toICAO {
			break
		}
		if cur.dist > dist[cur.icao] {
			continue
		}
		curAP, ok := airports[cur.icao]
		if !ok {
			continue
		}
		// Expand neighbors (all airports connected by a route)
		seen := make(map[string]bool)
		for _, rt := range byAirport[cur.icao] {
			parts := strings.Split(rt.AirportCodes, "-")
			for _, p := range parts {
				if p == cur.icao || p == "" || seen[p] {
					continue
				}
				seen[p] = true
				destAP, ok := airports[p]
				if !ok {
					continue
				}
				nd := cur.dist + haversineNM(curAP.Lat, curAP.Lon, destAP.Lat, destAP.Lon)
				if old, exists := dist[p]; !exists || nd < old {
					dist[p] = nd
					prev[p] = cur.icao
					heap.Push(h, &airPQItem{icao: p, dist: nd})
				}
			}
		}
	}

	if _, ok := dist[toICAO]; !ok {
		// No route found, fall back to direct
		return []string{fromICAO, toICAO}
	}

	// Reconstruct path
	var path []string
	for cur := toICAO; cur != ""; cur = prev[cur] {
		path = append(path, cur)
		if cur == fromICAO {
			break
		}
	}
	// Reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// Priority queue for air routing
type airPQItem struct {
	icao string
	dist float64
	idx  int
}
type airPQ []*airPQItem

func (h airPQ) Len() int            { return len(h) }
func (h airPQ) Less(i, j int) bool  { return h[i].dist < h[j].dist }
func (h airPQ) Swap(i, j int)       { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h *airPQ) Push(x any)         { it := x.(*airPQItem); it.idx = len(*h); *h = append(*h, it) }
func (h *airPQ) Pop() any           { old := *h; it := old[len(old)-1]; *h = old[:len(old)-1]; return it }

func nearestNode(lon, lat float64) (nodeID, float64) {
	bestDist := math.MaxFloat64
	bestNode := nodeID(-1)
	cx, cy := int(math.Floor(lon)), int(math.Floor(lat))
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			for _, nid := range marnetGrid[[2]int{cx + dx, cy + dy}] {
				d := haversineNM(lat, lon, marnetNodes[nid][1], marnetNodes[nid][0])
				if d < bestDist {
					bestDist = d
					bestNode = nid
				}
			}
		}
	}
	return bestNode, bestDist
}

// Priority queue for Dijkstra
type pqItem struct {
	node nodeID
	dist float64
	idx  int
}
type pq []*pqItem

func (h pq) Len() int            { return len(h) }
func (h pq) Less(i, j int) bool  { return h[i].dist < h[j].dist }
func (h pq) Swap(i, j int)       { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h *pq) Push(x any)         { it := x.(*pqItem); it.idx = len(*h); *h = append(*h, it) }
func (h *pq) Pop() any           { old := *h; it := old[len(old)-1]; *h = old[:len(old)-1]; return it }

// Dijkstra returns shortest path as polyline coords and total distance in nm
func dijkstraRoute(from, to nodeID) ([][2]float64, float64, []string) {
	n := len(marnetNodes)
	dist := make([]float64, n)
	prev := make([]nodeID, n)
	prevEdge := make([]int, n) // index into adj list for path reconstruction
	for i := range dist {
		dist[i] = math.MaxFloat64
		prev[i] = -1
	}
	dist[from] = 0

	h := &pq{{node: from, dist: 0}}
	heap.Init(h)

	for h.Len() > 0 {
		cur := heap.Pop(h).(*pqItem)
		if cur.node == to {
			break
		}
		if cur.dist > dist[cur.node] {
			continue
		}
		for ei, e := range marnetAdj[cur.node] {
			nd := cur.dist + e.dist
			if nd < dist[e.to] {
				dist[e.to] = nd
				prev[e.to] = cur.node
				prevEdge[e.to] = ei
				heap.Push(h, &pqItem{node: e.to, dist: nd})
			}
		}
	}

	if dist[to] == math.MaxFloat64 {
		return nil, 0, nil
	}

	// Reconstruct path + collect passes
	var path [][2]float64
	seen := make(map[string]bool)
	var passes []string
	for cur := to; cur != from; cur = prev[cur] {
		edge := marnetAdj[prev[cur]][prevEdge[cur]]
		if edge.pass != "" && !seen[edge.pass] {
			seen[edge.pass] = true
			passes = append(passes, edge.pass)
		}
		for i := len(edge.coords) - 1; i >= 1; i-- {
			path = append(path, edge.coords[i])
		}
	}
	// Add start node
	path = append(path, marnetNodes[from])
	// Reverse to get from→to order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	// Reverse passes (collected backwards)
	for i, j := 0, len(passes)-1; i < j; i, j = i+1, j-1 {
		passes[i], passes[j] = passes[j], passes[i]
	}
	return path, dist[to], passes
}


func loadShips(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.Read()
	ships = make(map[string]*Ship, 770000)
	shipsByCallSign = make(map[string]*Ship, 700000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 19 {
			continue
		}
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
		s := &Ship{
			MMSI: rec[0], IMO: rec[1], CallSign: rec[2], Name: rec[4], Country: rec[5],
			CountryCode: ituCountryCode(rec[5]),
			GrossTonnage: gt, ShipType: st, LengthM: ln, BeamM: bm, Class: cls,
		}
		ships[rec[0]] = s
		if rec[2] != "" {
			shipsByCallSign[strings.ToUpper(rec[2])] = s
		}
	}
	// Build sorted list for pagination
	shipsList = make([]*Ship, 0, len(ships))
	shipsByIMO = make(map[string]*Ship, 15000)
	for _, s := range ships {
		shipsList = append(shipsList, s)
		if s.IMO != "" {
			shipsByIMO[s.IMO] = s
		}
	}
	sort.Slice(shipsList, func(i, j int) bool { return shipsList[i].GrossTonnage > shipsList[j].GrossTonnage })
	return nil
}

func loadCompanies(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // optional file
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.Read() // header
	companyByCode = make(map[string]*ShippingCompany, 120)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 11 {
			continue
		}
		fleet, _ := strconv.Atoi(rec[6])
		teu, _ := strconv.Atoi(rec[7])
		c := &ShippingCompany{
			Code: rec[0], Name: rec[1], FullName: rec[2], CountryCode: rec[3],
			Sector: rec[4], Parent: rec[5], FleetSize: fleet, TEUCapacity: teu,
			Website: rec[8], NamePrefix: rec[9], Active: rec[10] == "1",
		}
		if len(rec) > 11 {
			c.IMOCompany = rec[11]
		}
		companies = append(companies, c)
		companyByCode[c.Code] = c
		if c.NamePrefix != "" {
			companyPrefixes = append(companyPrefixes, struct{ prefix, code string }{strings.ToUpper(c.NamePrefix), c.Code})
		}
	}
	// Sort prefixes longest first for greedy match
	sort.Slice(companyPrefixes, func(i, j int) bool {
		return len(companyPrefixes[i].prefix) > len(companyPrefixes[j].prefix)
	})
	return nil
}

func matchOperator(shipName string) *ShippingCompany {
	upper := strings.ToUpper(shipName)
	for _, p := range companyPrefixes {
		if strings.HasPrefix(upper, p.prefix) {
			return companyByCode[p.code]
		}
	}
	return nil
}

func loadNotableShips(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // optional
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.Read() // header: imo,mmsi,name,flag,ship_type,dwt,gt,teu,length_m,beam_m,year_built,builder,operator,sector,status,photo1,photo2
	notableByMMSI = make(map[string]*NotableShip, 10000)
	notableByName = make(map[string]*NotableShip, 10000)
	notableByIMO = make(map[string]*NotableShip, 10000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) < 15 {
			continue
		}
		dwt, _ := strconv.Atoi(rec[5])
		gt, _ := strconv.Atoi(rec[6])
		teu, _ := strconv.Atoi(rec[7])
		ln, _ := strconv.Atoi(strings.Split(rec[8], ".")[0])
		bm, _ := strconv.Atoi(strings.Split(rec[9], ".")[0])
		yr, _ := strconv.Atoi(rec[10])
		ns := &NotableShip{
			IMO: rec[0], MMSI: rec[1], Name: rec[2], Flag: rec[3],
			ShipType: rec[4], DWT: dwt, GT: gt, TEU: teu,
			LengthM: ln, BeamM: bm, YearBuilt: yr,
			Builder: rec[11], Operator: rec[12],
			Sector: rec[13], Status: rec[14],
		}
		if len(rec) > 15 { ns.Photo1 = rec[15] }
		if len(rec) > 16 { ns.Photo2 = rec[16] }
		notableShips = append(notableShips, ns)
		if ns.MMSI != "" {
			notableByMMSI[ns.MMSI] = ns
		}
		if ns.IMO != "" {
			notableByIMO[ns.IMO] = ns
		}
		notableByName[strings.ToUpper(ns.Name)] = ns
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

// ===================== WEBSOCKET HELPERS =====================

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-5AB0A17FE6E5"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func writeWSFrame(conn net.Conn, payload []byte) {
	// Binary frame, FIN=1, opcode=2
	hdr := []byte{0x82}
	n := len(payload)
	if n < 126 {
		hdr = append(hdr, byte(n))
	} else if n < 65536 {
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	} else {
		hdr = append(hdr, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	conn.Write(hdr)
	conn.Write(payload)
}

func readWSFrame(conn net.Conn) ([]byte, error) {
	hdr := make([]byte, 2)
	if _, err := conn.Read(hdr); err != nil {
		return nil, err
	}
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7F)
	if length == 126 {
		ext := make([]byte, 2)
		conn.Read(ext)
		length = int(ext[0])<<8 | int(ext[1])
	} else if length == 127 {
		ext := make([]byte, 8)
		conn.Read(ext)
		length = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
	}
	var mask [4]byte
	if masked {
		conn.Read(mask[:])
	}
	data := make([]byte, length)
	if length > 0 {
		total := 0
		for total < length {
			n, err := conn.Read(data[total:])
			if err != nil {
				return nil, err
			}
			total += n
		}
		if masked {
			for i := range data {
				data[i] ^= mask[i%4]
			}
		}
	}
	return data, nil
}

func buildPortsBinary() []byte {
	strMap := make(map[string]uint16)
	var strList []string
	addStr := func(s string) uint16 {
		if idx, ok := strMap[s]; ok {
			return idx
		}
		idx := uint16(len(strList))
		strMap[s] = idx
		strList = append(strList, s)
		return idx
	}
	sizeMap := map[string]uint8{"Major": 5, "Large": 4, "Medium": 3, "Small": 2, "Minor": 1, "Very Small": 0}
	type bp struct{ lat, lon int32; size uint8; ni, ci uint16; flags uint8; teu uint16 }
	pts := make([]bp, len(seaports))
	for i := range seaports {
		p := &seaports[i]
		cs := p.CountryCode
		pts[i] = bp{int32(p.Lat * 1e6), int32(p.Lon * 1e6), sizeMap[p.PortSize], addStr(p.Name), addStr(cs), 0, uint16(p.TEUThousands)}
		if p.LOCODE != "" {
			pts[i].flags |= 0x01
		}
	}
	stSize := 2
	for _, s := range strList {
		stSize += 2 + len(s)
	}
	buf := make([]byte, 16+len(pts)*16+stSize)
	copy(buf[0:4], "HPRA")
	buf[4] = 1; buf[5] = 1
	binary.LittleEndian.PutUint16(buf[6:8], uint16(len(pts)))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(16+len(pts)*16))
	off := 16
	for _, pt := range pts {
		binary.LittleEndian.PutUint32(buf[off:], uint32(pt.lat))
		binary.LittleEndian.PutUint32(buf[off+4:], uint32(pt.lon))
		buf[off+8] = pt.size
		binary.LittleEndian.PutUint16(buf[off+9:], pt.ni)
		binary.LittleEndian.PutUint16(buf[off+11:], pt.ci)
		buf[off+13] = pt.flags
		binary.LittleEndian.PutUint16(buf[off+14:], pt.teu)
		off += 16
	}
	binary.LittleEndian.PutUint16(buf[off:], uint16(len(strList)))
	off += 2
	for _, s := range strList {
		binary.LittleEndian.PutUint16(buf[off:], uint16(len(s)))
		off += 2
		copy(buf[off:], s)
		off += len(s)
	}
	return buf[:off]
}

func buildAirportsBinary() []byte {
	strMap := make(map[string]uint16)
	var strList []string
	addStr := func(s string) uint16 {
		if idx, ok := strMap[s]; ok {
			return idx
		}
		idx := uint16(len(strList))
		strMap[s] = idx
		strList = append(strList, s)
		return idx
	}
	type ba struct{ lat, lon int32; ni, ii, ai, rc uint16 }
	pts := make([]ba, 0, len(airports))
	for _, a := range airports {
		nameStr := a.Name
		if a.CountryCode != "" {
			nameStr = a.Name + "|" + a.CountryCode
		}
		pts = append(pts, ba{int32(a.Lat * 1e6), int32(a.Lon * 1e6), addStr(nameStr), addStr(a.ICAO), addStr(a.IATA), uint16(len(byAirport[a.ICAO]))})
	}
	stSize := 2
	for _, s := range strList {
		stSize += 2 + len(s)
	}
	buf := make([]byte, 16+len(pts)*16+stSize)
	copy(buf[0:4], "HPRA")
	buf[4] = 1; buf[5] = 2
	binary.LittleEndian.PutUint16(buf[6:8], uint16(len(pts)))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(16+len(pts)*16))
	off := 16
	for _, pt := range pts {
		binary.LittleEndian.PutUint32(buf[off:], uint32(pt.lat))
		binary.LittleEndian.PutUint32(buf[off+4:], uint32(pt.lon))
		binary.LittleEndian.PutUint16(buf[off+8:], pt.ni)
		binary.LittleEndian.PutUint16(buf[off+10:], pt.ii)
		binary.LittleEndian.PutUint16(buf[off+12:], pt.ai)
		binary.LittleEndian.PutUint16(buf[off+14:], pt.rc)
		off += 16
	}
	binary.LittleEndian.PutUint16(buf[off:], uint16(len(strList)))
	off += 2
	for _, s := range strList {
		binary.LittleEndian.PutUint16(buf[off:], uint16(len(s)))
		off += 2
		copy(buf[off:], s)
		off += len(s)
	}
	return buf[:off]
}

// ===================== MAIN =====================

func main() {
	for _, l := range []struct {
		name string
		fn   func(string) error
		path string
	}{
		{"aircraft", loadAircraft, "data/aircraft.csv"},
		{"airlines", loadAirlines, "data/airlines.csv"},
		{"airports", loadAirports, "data/airports.csv"},
		{"routes", loadRoutes, "data/routes.csv"},
		{"seaports", loadSeaports, "data/seaports.csv"},
		{"sea_routes", loadSeaRoutes, "data/sea_distances.csv"},
		{"sea_distance_ports", loadSeaDistancePorts, "data/sea_distance_ports.csv"},
		{"shipping_lanes", loadShippingLanes, "data/shipping_lanes.geojson"},
		{"marnet", loadMarnet, "data/marnet.geojson"},
		{"ships", loadShips, "data/ships.csv"},
		{"companies", loadCompanies, "data/shipping_companies.csv"},
		{"notable_ships", loadNotableShips, "data/notable_ships.csv"},
	} {
		if err := l.fn(l.path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load %s: %v\n", l.name, err)
			os.Exit(1)
		}
	}
	startTime = time.Now()

	// Build combined shipping lanes: CIA (tiers 1-3) + marnet local detail (tier 4)
	var ciaGJ struct {
		Features []json.RawMessage `json:"features"`
	}
	json.Unmarshal(shippingLanesJSON, &ciaGJ)
	tierMap := map[string]int{"Major": 1, "Middle": 2, "Minor": 3}
	var combinedFeats []map[string]any
	for _, raw := range ciaGJ.Features {
		var f map[string]any
		json.Unmarshal(raw, &f)
		props, _ := f["properties"].(map[string]any)
		if props != nil {
			if t, ok := props["Type"].(string); ok {
				props["tier"] = tierMap[t]
			}
		}
		combinedFeats = append(combinedFeats, f)
	}
	// Append marnet tier-4 features
	var marnetGJ struct {
		Features []json.RawMessage `json:"features"`
	}
	json.Unmarshal(marnetJSON, &marnetGJ)
	for _, raw := range marnetGJ.Features {
		var f map[string]any
		json.Unmarshal(raw, &f)
		combinedFeats = append(combinedFeats, f)
	}
	combinedLanesJSON, _ = json.Marshal(map[string]any{"type": "FeatureCollection", "features": combinedFeats})
	fmt.Printf("  combined lanes: %d CIA + %d marnet-local features\n", len(ciaGJ.Features), len(marnetGJ.Features))

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
			"api_version": Version,
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
			"version": Version,
			"aviation": map[string]any{
				"aircraft": len(aircraft),
				"routes":   len(routes),
				"airlines": len(byAirline),
				"airports": len(airports),
			},
			"maritime": map[string]any{
				"ships":            len(ships),
				"seaports":         len(seaports),
				"sea_route_origins": len(seaRoutesByOrigin),
				"companies":        len(companies),
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
			"airport": code, "total_flights": len(rts),
			"destinations": len(connected), "top_connections": conns,
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
		id := r.URL.Path[len("/v1/ports/"):]
		idU := strings.ToUpper(id)
		if p, ok := portByLOCODE[idU]; ok {
			writeJSON(w, p)
			return
		}
		if p, ok := portByWPI[id]; ok {
			writeJSON(w, p)
			return
		}
		// Fallback: name match
		idN := strings.ReplaceAll(strings.ReplaceAll(idU, " ", ""), "-", "")
		for i := range seaports {
			n := strings.ToUpper(seaports[i].Name)
			if n == idU || strings.Contains(strings.ReplaceAll(strings.ReplaceAll(n, " ", ""), "-", ""), idN) {
				writeJSON(w, &seaports[i])
				return
			}
		}
		notFound(w, "Port not found")
	})

	// === Sea routes v1 ===

	mux.HandleFunc("/v1/sea-routes/from/", func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.URL.Path[len("/v1/sea-routes/from/"):])
		key := strings.ToUpper(origin)
		rts := seaRoutesByOrigin[key]
		if len(rts) == 0 {
			keyNoSpc := strings.ReplaceAll(strings.ReplaceAll(key, " ", ""), "-", "")
			for k, v := range seaRoutesByOrigin {
				ku := strings.ToUpper(k)
				if strings.Contains(ku, key) || strings.Contains(strings.ReplaceAll(strings.ReplaceAll(ku, " ", ""), "-", ""), keyNoSpc) {
					rts = v
					break
				}
			}
		}
		// Dijkstra fallback: compute routes to major ports via marnet graph
		if len(rts) == 0 {
			var port *Seaport
			qu := strings.ToUpper(strings.TrimSpace(origin))
			qn := strings.ReplaceAll(strings.ReplaceAll(qu, " ", ""), "-", "")
			for i := range seaports {
				n := strings.ToUpper(seaports[i].Name)
				if n == qu || strings.Contains(n, qu) || strings.Contains(strings.ReplaceAll(strings.ReplaceAll(n, " ", ""), "-", ""), qn) {
					port = &seaports[i]
					break
				}
			}
			if port != nil {
				srcNode, srcDist := nearestNode(port.Lon, port.Lat)
				if srcNode >= 0 && srcDist < 200 {
					// Compute distances to top ports by TEU
					for i := range seaports {
						dp := &seaports[i]
						if dp.TEUThousands == 0 || dp.Name == port.Name {
							continue
						}
						dstNode, dstDist := nearestNode(dp.Lon, dp.Lat)
						if dstNode < 0 || dstDist > 200 {
							continue
						}
						_, dist, _ := dijkstraRoute(srcNode, dstNode)
						if dist > 0 {
							rts = append(rts, SeaRoute{Origin: port.Name, Destination: dp.Name + " (" + dp.CountryCode + ")", DistanceNM: math.Round(dist), Type: "port"})
						}
						if len(rts) >= 30 {
							break
						}
					}
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

	mux.HandleFunc("/v1/sea-routes/geojson", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		if from == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"from parameter required"}`))
			return
		}
		key := strings.ToUpper(from)
		origin, ok := seaDistancePorts[key]
		if !ok {
			// Fallback: partial match (also try without spaces)
			keyNoSpc := strings.ReplaceAll(key, " ", "")
			for k, v := range seaDistancePorts {
				if strings.Contains(k, key) || strings.Contains(strings.ReplaceAll(k, " ", ""), keyNoSpc) {
					origin = v
					key = k
					ok = true
					break
				}
			}
		}
		if !ok {
			notFound(w, "Origin port not found in distance ports")
			return
		}
		rts := seaRoutesByOrigin[key]
		if len(rts) == 0 {
			keyNoSpc := strings.ReplaceAll(key, " ", "")
			for k, v := range seaRoutesByOrigin {
				ku := strings.ToUpper(k)
				if strings.Contains(ku, key) || strings.Contains(strings.ReplaceAll(ku, " ", ""), keyNoSpc) {
					rts = v
					break
				}
			}
		}
		if len(rts) == 0 {
			notFound(w, "No routes from this origin")
			return
		}
		type feat struct {
			Type string         `json:"type"`
			Geom map[string]any `json:"geometry"`
			Prop map[string]any `json:"properties"`
		}
		var features []feat
		for _, rt := range rts {
			dest, ok := seaDistancePorts[strings.ToUpper(rt.Destination)]
			if !ok {
				continue
			}
			features = append(features, feat{
				Type: "Feature",
				Geom: map[string]any{"type": "LineString", "coordinates": [][]float64{{origin.Lon, origin.Lat}, {dest.Lon, dest.Lat}}},
				Prop: map[string]any{"origin": rt.Origin, "destination": rt.Destination, "distance_nm": rt.DistanceNM, "type": rt.Type},
			})
		}
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": features})
	})

	mux.HandleFunc("/v1/shipping-lanes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(combinedLanesJSON)
	})

	mux.HandleFunc("/v1/shipping-lanes/legacy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(shippingLanesJSON)
	})

	// Sea route (Dijkstra on marnet graph)
	mux.HandleFunc("/v1/sea-routes/route", func(w http.ResponseWriter, r *http.Request) {
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		if fromStr == "" || toStr == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"from and to parameters required (lat,lon)"}`))
			return
		}
		parsePt := func(s string) (float64, float64, bool) {
			parts := strings.SplitN(s, ",", 2)
			if len(parts) != 2 {
				return 0, 0, false
			}
			lat, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if e1 != nil || e2 != nil {
				return 0, 0, false
			}
			return lat, lon, true
		}
		fromLat, fromLon, ok1 := parsePt(fromStr)
		toLat, toLon, ok2 := parsePt(toStr)
		if !ok1 || !ok2 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid coordinates, use lat,lon format"}`))
			return
		}
		srcNode, srcDist := nearestNode(fromLon, fromLat)
		dstNode, dstDist := nearestNode(toLon, toLat)
		if srcNode < 0 || dstNode < 0 || srcDist > 200 || dstDist > 200 {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no graph node within 200nm of given coordinates"}`))
			return
		}
		path, dist, _ := dijkstraRoute(srcNode, dstNode)
		if path == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no route found between these points"}`))
			return
		}
		// Build GeoJSON LineString
		coords := make([][]float64, len(path))
		for i, p := range path {
			coords[i] = []float64{p[0], p[1]} // already [lon,lat]
		}
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": map[string]any{
				"distance_nm":   math.Round(dist*10) / 10,
				"from_snap_nm":  math.Round(srcDist*10) / 10,
				"to_snap_nm":    math.Round(dstDist*10) / 10,
				"nodes_in_path": len(path),
			},
		})
	})

	// === Unified Route Engine ===

	// Resolve port by name, LOCODE, or WPI ID
	resolvePort := func(q string) *Seaport {
		q = strings.TrimSpace(q)
		qu := strings.ToUpper(q)
		qn := strings.ReplaceAll(strings.ReplaceAll(qu, " ", ""), "-", "")
		// Try LOCODE via index
		if p, ok := portByLOCODE[qu]; ok {
			return p
		}
		// Try LOCODE in records
		for i := range seaports {
			if strings.ToUpper(seaports[i].LOCODE) == qu {
				return &seaports[i]
			}
		}
		// Try WPI ID
		if p, ok := portByWPI[q]; ok {
			return p
		}
		// Try name exact
		for i := range seaports {
			if strings.ToUpper(seaports[i].Name) == qu {
				return &seaports[i]
			}
		}
		// Fuzzy: contains or space-stripped match
		for i := range seaports {
			name := strings.ToUpper(seaports[i].Name)
			if strings.Contains(name, qu) {
				return &seaports[i]
			}
			nn := strings.ReplaceAll(strings.ReplaceAll(name, " ", ""), "-", "")
			if strings.Contains(nn, qn) || strings.Contains(qn, nn) {
				return &seaports[i]
			}
		}
		return nil
	}

	// Resolve airport by ICAO or IATA
	resolveAirport := func(q string) *Airport {
		qu := strings.ToUpper(strings.TrimSpace(q))
		if ap, ok := airports[qu]; ok {
			return &ap
		}
		// Try IATA
		for _, ap := range airports {
			if strings.ToUpper(ap.IATA) == qu {
				a := ap
				return &a
			}
		}
		// Fuzzy name
		for _, ap := range airports {
			if strings.Contains(strings.ToUpper(ap.Name), qu) {
				a := ap
				return &a
			}
		}
		return nil
	}

	mux.HandleFunc("/v1/routes/sea", func(w http.ResponseWriter, r *http.Request) {
		fromQ := r.URL.Query().Get("from")
		toQ := r.URL.Query().Get("to")
		if fromQ == "" || toQ == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"from and to parameters required (port name or LOCODE)"}`))
			return
		}
		fromPort := resolvePort(fromQ)
		toPort := resolvePort(toQ)
		if fromPort == nil {
			notFound(w, "Origin port not found: "+fromQ)
			return
		}
		if toPort == nil {
			notFound(w, "Destination port not found: "+toQ)
			return
		}
		srcNode, srcDist := nearestNode(fromPort.Lon, fromPort.Lat)
		dstNode, dstDist := nearestNode(toPort.Lon, toPort.Lat)
		if srcNode < 0 || dstNode < 0 || srcDist > 200 || dstDist > 200 {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"ports too far from maritime network"}`))
			return
		}
		path, distNM, passes := dijkstraRoute(srcNode, dstNode)
		if path == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no sea route found"}`))
			return
		}
		speedKn := 14.0
		if s := r.URL.Query().Get("speed_kn"); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				speedKn = v
			}
		}
		coords := make([][]float64, len(path))
		for i, p := range path {
			coords[i] = []float64{p[0], p[1]}
		}
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{
			"type":     "Feature",
			"geometry": map[string]any{"type": "LineString", "coordinates": coords},
			"properties": map[string]any{
				"mode": "sea",
				"from": map[string]any{"name": fromPort.Name, "code": fromPort.LOCODE, "country_code": fromPort.CountryCode, "lat": fromPort.Lat, "lon": fromPort.Lon, "teu_thousands": fromPort.TEUThousands},
				"to":   map[string]any{"name": toPort.Name, "code": toPort.LOCODE, "country_code": toPort.CountryCode, "lat": toPort.Lat, "lon": toPort.Lon, "teu_thousands": toPort.TEUThousands},
				"distance_nm":    math.Round(distNM*10) / 10,
				"distance_km":    math.Round(distNM*1.852*10) / 10,
				"speed_kn":       speedKn,
				"estimated_hours": math.Round(distNM/speedKn*10) / 10,
				"passes":         passes,
				"nodes_in_path":  len(path),
			},
		})
	})

	mux.HandleFunc("/v1/routes/air", func(w http.ResponseWriter, r *http.Request) {
		fromQ := r.URL.Query().Get("from")
		toQ := r.URL.Query().Get("to")
		if fromQ == "" || toQ == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"from and to parameters required (ICAO or IATA)"}`))
			return
		}
		fromAP := resolveAirport(fromQ)
		toAP := resolveAirport(toQ)
		if fromAP == nil {
			notFound(w, "Origin airport not found: "+fromQ)
			return
		}
		if toAP == nil {
			notFound(w, "Destination airport not found: "+toQ)
			return
		}
		speedKn := 480.0
		if s := r.URL.Query().Get("speed_kn"); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				speedKn = v
			}
		}

		// Find multi-hop route via BFS/Dijkstra on flight network
		hops := findAirRoute(fromAP.ICAO, toAP.ICAO)

		// Build concatenated great-circle arcs through hops
		var allCoords [][]float64
		var totalDist float64
		type hopInfo struct {
			Code string `json:"code"`
			IATA string `json:"iata"`
			Name string `json:"name"`
			CC   string `json:"country_code"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
		}
		type segInfo struct {
			From     string   `json:"from"`
			To       string   `json:"to"`
			Dist     float64  `json:"distance_nm"`
			Airlines []string `json:"airlines"`
		}
		var hopList []hopInfo
		var segments []segInfo

		for i, icao := range hops {
			ap, ok := airports[icao]
			if !ok {
				continue
			}
			hopList = append(hopList, hopInfo{icao, ap.IATA, ap.Name, ap.CountryCode, ap.Lat, ap.Lon})
			if i > 0 {
				prevAP := airports[hops[i-1]]
				segDist := haversineNM(prevAP.Lat, prevAP.Lon, ap.Lat, ap.Lon)
				totalDist += segDist
				arc := greatCircleArc(prevAP.Lat, prevAP.Lon, ap.Lat, ap.Lon)
				// Skip first point of subsequent arcs to avoid duplicates
				start := 0
				if len(allCoords) > 0 {
					start = 1
				}
				for j := start; j < len(arc); j++ {
					allCoords = append(allCoords, arc[j])
				}
				// Find airlines for this segment
				var segAirlines []string
				seenAl := make(map[string]bool)
				pair1 := hops[i-1] + "-" + icao
				pair2 := icao + "-" + hops[i-1]
				for _, rt := range byAirport[hops[i-1]] {
					if strings.Contains(rt.AirportCodes, pair1) || strings.Contains(rt.AirportCodes, pair2) {
						if !seenAl[rt.AirlineCode] {
							seenAl[rt.AirlineCode] = true
							segAirlines = append(segAirlines, rt.AirlineCode)
							if len(segAirlines) >= 5 {
								break
							}
						}
					}
				}
				segments = append(segments, segInfo{hops[i-1], icao, math.Round(segDist*10) / 10, segAirlines})
			}
		}

		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{
			"type":     "Feature",
			"geometry": map[string]any{"type": "LineString", "coordinates": allCoords},
			"properties": map[string]any{
				"mode": "air",
				"from": map[string]any{"name": fromAP.Name, "code": fromAP.ICAO, "iata": fromAP.IATA, "country_code": fromAP.CountryCode, "lat": fromAP.Lat, "lon": fromAP.Lon},
				"to":   map[string]any{"name": toAP.Name, "code": toAP.ICAO, "iata": toAP.IATA, "country_code": toAP.CountryCode, "lat": toAP.Lat, "lon": toAP.Lon},
				"distance_nm":    math.Round(totalDist*10) / 10,
				"distance_km":    math.Round(totalDist*1.852*10) / 10,
				"speed_kn":       speedKn,
				"estimated_hours": math.Round(totalDist/speedKn*10) / 10,
				"hops":           hopList,
				"segments":       segments,
				"arc_points":     len(allCoords),
			},
		})
	})

	mux.HandleFunc("/v1/routes/sea/voyage", func(w http.ResponseWriter, r *http.Request) {
		latS := r.URL.Query().Get("lat")
		lonS := r.URL.Query().Get("lon")
		toQ := r.URL.Query().Get("to")
		if latS == "" || lonS == "" || toQ == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"lat, lon, and to parameters required"}`))
			return
		}
		lat, _ := strconv.ParseFloat(latS, 64)
		lon, _ := strconv.ParseFloat(lonS, 64)
		toPort := resolvePort(toQ)
		if toPort == nil {
			notFound(w, "Destination port not found: "+toQ)
			return
		}
		speedKn := 14.0
		if s := r.URL.Query().Get("speed_kn"); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				speedKn = v
			}
		}
		srcNode, _ := nearestNode(lon, lat)
		dstNode, _ := nearestNode(toPort.Lon, toPort.Lat)
		if srcNode < 0 || dstNode < 0 {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"position or port too far from maritime network"}`))
			return
		}
		path, remainNM, passes := dijkstraRoute(srcNode, dstNode)
		if path == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no route found"}`))
			return
		}
		coords := make([][]float64, len(path))
		for i, p := range path {
			coords[i] = []float64{p[0], p[1]}
		}
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{
			"type":     "Feature",
			"geometry": map[string]any{"type": "LineString", "coordinates": coords},
			"properties": map[string]any{
				"mode":             "sea",
				"destination":      map[string]any{"name": toPort.Name, "code": toPort.LOCODE, "country_code": toPort.CountryCode},
				"current_position": []float64{lon, lat},
				"remaining_nm":     math.Round(remainNM*10) / 10,
				"remaining_km":     math.Round(remainNM*1.852*10) / 10,
				"speed_kn":         speedKn,
				"eta_hours":        math.Round(remainNM/speedKn*10) / 10,
				"passes_remaining": passes,
				"nodes_in_path":    len(path),
			},
		})
	})

	mux.HandleFunc("/v1/routes/air/voyage", func(w http.ResponseWriter, r *http.Request) {
		latS := r.URL.Query().Get("lat")
		lonS := r.URL.Query().Get("lon")
		toQ := r.URL.Query().Get("to")
		if latS == "" || lonS == "" || toQ == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"lat, lon, and to parameters required"}`))
			return
		}
		lat, _ := strconv.ParseFloat(latS, 64)
		lon, _ := strconv.ParseFloat(lonS, 64)
		toAP := resolveAirport(toQ)
		if toAP == nil {
			notFound(w, "Destination airport not found: "+toQ)
			return
		}
		speedKn := 480.0
		if s := r.URL.Query().Get("speed_kn"); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
				speedKn = v
			}
		}
		remainNM := haversineNM(lat, lon, toAP.Lat, toAP.Lon)
		arc := greatCircleArc(lat, lon, toAP.Lat, toAP.Lon)
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{
			"type":     "Feature",
			"geometry": map[string]any{"type": "LineString", "coordinates": arc},
			"properties": map[string]any{
				"mode":             "air",
				"destination":      map[string]any{"name": toAP.Name, "code": toAP.ICAO, "iata": toAP.IATA, "country_code": toAP.CountryCode},
				"current_position": []float64{lon, lat},
				"remaining_nm":     math.Round(remainNM*10) / 10,
				"remaining_km":     math.Round(remainNM*1.852*10) / 10,
				"speed_kn":         speedKn,
				"eta_hours":        math.Round(remainNM/speedKn*10) / 10,
				"arc_points":       len(arc),
			},
		})
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
		id := r.URL.Path[len("/v1/ships/"):]
		// Try MMSI first
		if s, ok := ships[id]; ok {
			resp := map[string]any{
				"mmsi": s.MMSI, "imo": s.IMO, "call_sign": s.CallSign, "name": s.Name,
				"country": s.Country, "country_code": s.CountryCode,
				"gross_tonnage": s.GrossTonnage, "ship_type": s.ShipType,
				"length_m": s.LengthM, "beam_m": s.BeamM, "class": s.Class,
			}
			if op := matchOperator(s.Name); op != nil {
				resp["operator"] = map[string]any{"code": op.Code, "name": op.Name, "country_code": op.CountryCode, "sector": op.Sector}
			}
			if ns, ok := notableByMMSI[id]; ok {
				resp["notable"] = ns
			} else if ns, ok := notableByName[strings.ToUpper(s.Name)]; ok {
				resp["notable"] = ns
			}
			writeJSON(w, resp)
			return
		}
		// Try IMO (notable ships)
		if ns, ok := notableByIMO[id]; ok {
			resp := map[string]any{
				"imo": ns.IMO, "mmsi": ns.MMSI, "name": ns.Name, "flag": ns.Flag,
				"ship_type": ns.ShipType, "dwt": ns.DWT, "gt": ns.GT, "teu": ns.TEU,
				"length_m": ns.LengthM, "beam_m": ns.BeamM, "year_built": ns.YearBuilt,
				"builder": ns.Builder, "operator": ns.Operator, "sector": ns.Sector,
			}
			// Also pull ITU data if MMSI exists
			if s, ok := ships[ns.MMSI]; ok {
				resp["call_sign"] = s.CallSign
				resp["country"] = s.Country
				resp["country_code"] = s.CountryCode
				resp["gross_tonnage"] = s.GrossTonnage
			}
			writeJSON(w, resp)
			return
		}
		// Try IMO in ITU (enriched from MongoDB AIS)
		if s, ok := shipsByIMO[id]; ok {
			resp := map[string]any{
				"mmsi": s.MMSI, "imo": s.IMO, "call_sign": s.CallSign, "name": s.Name,
				"country": s.Country, "country_code": s.CountryCode,
				"gross_tonnage": s.GrossTonnage, "ship_type": s.ShipType,
				"length_m": s.LengthM, "beam_m": s.BeamM, "class": s.Class,
			}
			if op := matchOperator(s.Name); op != nil {
				resp["operator"] = map[string]any{"code": op.Code, "name": op.Name, "country_code": op.CountryCode, "sector": op.Sector}
			}
			writeJSON(w, resp)
			return
		}
		notFound(w, "Ship not found")
	})

	// Notable ships endpoint
	mux.HandleFunc("/v1/ships/notable", func(w http.ResponseWriter, r *http.Request) {
		sector := strings.ToUpper(r.URL.Query().Get("sector"))
		var result []*NotableShip
		for _, ns := range notableShips {
			if sector != "" && ns.Sector != sector {
				continue
			}
			result = append(result, ns)
		}
		writeJSON(w, result)
	})

	// Full ship registry (746k) — paginated, sortable, filterable
	mux.HandleFunc("/v1/ships/list", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := qInt(r, "limit", 50, 500)
		offset := qInt(r, "offset", 0, len(shipsList))
		sortKey := q.Get("sort")
		if sortKey == "" {
			sortKey = "gt"
		}
		desc := q.Get("order") != "asc"
		countryFilter := strings.ToUpper(q.Get("country"))
		nameFilter := strings.ToUpper(q.Get("q"))

		// Filter
		var filtered []*Ship
		// Check if query matches a notable ship IMO — inject at top
		if nameFilter != "" {
			if ns, ok := notableByIMO[strings.TrimSpace(string(nameFilter))]; ok && ns.MMSI != "" {
				if s, ok2 := ships[ns.MMSI]; ok2 {
					filtered = append(filtered, s)
				}
			}
		}
		for _, s := range shipsList {
			if countryFilter != "" && s.CountryCode != countryFilter {
				continue
			}
			if nameFilter != "" && !strings.Contains(strings.ToUpper(s.Name), nameFilter) && !strings.Contains(s.MMSI, nameFilter) && !strings.Contains(strings.ToUpper(s.CallSign), nameFilter) && !strings.Contains(s.IMO, nameFilter) {
				continue
			}
			filtered = append(filtered, s)
		}

		// Sort
		sort.Slice(filtered, func(i, j int) bool {
			switch sortKey {
			case "name":
				if desc {
					return filtered[i].Name > filtered[j].Name
				}
				return filtered[i].Name < filtered[j].Name
			case "length":
				if desc {
					return filtered[i].LengthM > filtered[j].LengthM
				}
				return filtered[i].LengthM < filtered[j].LengthM
			case "mmsi":
				if desc {
					return filtered[i].MMSI > filtered[j].MMSI
				}
				return filtered[i].MMSI < filtered[j].MMSI
			default: // gt
				if desc {
					return filtered[i].GrossTonnage > filtered[j].GrossTonnage
				}
				return filtered[i].GrossTonnage < filtered[j].GrossTonnage
			}
		})

		total := len(filtered)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		writeJSON(w, map[string]any{
			"total": total, "offset": offset, "limit": limit,
			"ships": filtered[offset:end],
		})
	})

	// === Companies v1 ===

	mux.HandleFunc("/v1/companies", func(w http.ResponseWriter, r *http.Request) {
		sector := r.URL.Query().Get("sector")
		var result []*ShippingCompany
		for _, c := range companies {
			if !c.Active {
				continue
			}
			if sector != "" && !strings.Contains(c.Sector, strings.ToUpper(sector)) {
				continue
			}
			result = append(result, c)
		}
		writeJSON(w, result)
	})

	mux.HandleFunc("/v1/companies/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/companies/"):])
		if c, ok := companyByCode[code]; ok {
			writeJSON(w, c)
		} else {
			notFound(w, "Company not found")
		}
	})

	// === Universal Search & Discovery ===

	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" || len(q) < 2 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"q parameter required (min 2 chars)"}`))
			return
		}
		qu := strings.ToUpper(q)
		qn := strings.ReplaceAll(qu, " ", "")
		limit := 20
		type result struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Code string `json:"code"`
			CC   string `json:"country_code,omitempty"`
			Lat  float64 `json:"lat,omitempty"`
			Lon  float64 `json:"lon,omitempty"`
		}
		var results []result
		// Ports
		for i := range seaports {
			if len(results) >= limit { break }
			p := &seaports[i]
			if !p.Active { continue }
			if strings.Contains(strings.ToUpper(p.Name), qu) || strings.Contains(p.LOCODE, qu) {
				results = append(results, result{"port", p.Name, p.LOCODE, p.CountryCode, p.Lat, p.Lon})
			}
		}
		// Airports
		for _, a := range airports {
			if len(results) >= limit { break }
			if strings.Contains(strings.ToUpper(a.Name), qu) || a.ICAO == qu || a.IATA == qu || strings.Contains(qn, strings.ReplaceAll(strings.ToUpper(a.Name), " ", "")) {
				results = append(results, result{"airport", a.Name, a.ICAO, a.Country, a.Lat, a.Lon})
			}
		}
		// Ships (by MMSI or name prefix)
		if len(results) < limit {
			if s, ok := ships[q]; ok {
				results = append(results, result{"ship", s.Name, s.MMSI, s.CountryCode, 0, 0})
			} else {
				count := 0
				for _, s := range ships {
					if count >= 5 { break }
					if strings.Contains(strings.ToUpper(s.Name), qu) {
						results = append(results, result{"ship", s.Name, s.MMSI, s.CountryCode, 0, 0})
						count++
					}
				}
			}
		}
		writeJSON(w, map[string]any{"query": q, "results": results})
	})

	mux.HandleFunc("/v1/ports/top", func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, _ := strconv.Atoi(l); v > 0 && v <= 200 { limit = v }
		}
		type portItem struct {
			LOCODE  string `json:"locode"`
			Name    string `json:"name"`
			CC      string `json:"country_code"`
			TEU     int    `json:"teu_thousands"`
			Size    string `json:"port_size"`
		}
		var items []portItem
		for i := range seaports {
			if seaports[i].TEUThousands > 0 {
				p := &seaports[i]
				items = append(items, portItem{p.LOCODE, p.Name, p.CountryCode, p.TEUThousands, p.PortSize})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].TEU > items[j].TEU })
		if len(items) > limit { items = items[:limit] }
		writeJSON(w, items)
	})

	mux.HandleFunc("/v1/airports/top", func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, _ := strconv.Atoi(l); v > 0 && v <= 200 { limit = v }
		}
		type apItem struct {
			ICAO   string `json:"icao"`
			IATA   string `json:"iata"`
			Name   string `json:"name"`
			CC     string `json:"country_code"`
			Routes int    `json:"routes"`
		}
		var items []apItem
		for icao, rts := range byAirport {
			if ap, ok := airports[icao]; ok {
				items = append(items, apItem{icao, ap.IATA, ap.Name, ap.Country, len(rts)})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Routes > items[j].Routes })
		if len(items) > limit { items = items[:limit] }
		writeJSON(w, items)
	})

	mux.HandleFunc("/v1/ports/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"q parameter required"}`))
			return
		}
		qu := strings.ToUpper(q)
		qn := strings.ReplaceAll(strings.ReplaceAll(qu, " ", ""), "-", "")
		var results []map[string]any
		for i := range seaports {
			if len(results) >= 20 { break }
			p := &seaports[i]
			if !p.Active { continue }
			n := strings.ToUpper(p.Name)
			if strings.Contains(n, qu) || strings.Contains(p.LOCODE, qu) || strings.Contains(strings.ReplaceAll(n, " ", ""), qn) {
				results = append(results, map[string]any{"locode": p.LOCODE, "name": p.Name, "country_code": p.CountryCode, "lat": p.Lat, "lon": p.Lon, "port_size": p.PortSize, "teu_thousands": p.TEUThousands})
			}
		}
		writeJSON(w, results)
	})

	mux.HandleFunc("/v1/airports/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"q parameter required"}`))
			return
		}
		qu := strings.ToUpper(q)
		var results []map[string]any
		for _, a := range airports {
			if len(results) >= 20 { break }
			if a.ICAO == qu || a.IATA == qu || strings.Contains(strings.ToUpper(a.Name), qu) || strings.Contains(strings.ToUpper(a.City), qu) {
				results = append(results, map[string]any{"icao": a.ICAO, "iata": a.IATA, "name": a.Name, "city": a.City, "country_code": a.Country, "lat": a.Lat, "lon": a.Lon, "type": a.Type, "routes": len(byAirport[a.ICAO])})
			}
		}
		writeJSON(w, results)
	})

	mux.HandleFunc("/v1/airports/nearby", func(w http.ResponseWriter, r *http.Request) {
		latS := r.URL.Query().Get("lat")
		lonS := r.URL.Query().Get("lon")
		if latS == "" || lonS == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"lat and lon required"}`))
			return
		}
		lat, _ := strconv.ParseFloat(latS, 64)
		lon, _ := strconv.ParseFloat(lonS, 64)
		radius := 100.0
		if rs := r.URL.Query().Get("radius_km"); rs != "" {
			if v, _ := strconv.ParseFloat(rs, 64); v > 0 { radius = v }
		}
		radiusNM := radius / 1.852
		limit := 20
		type nearby struct {
			ICAO string  `json:"icao"`
			IATA string  `json:"iata"`
			Name string  `json:"name"`
			CC   string  `json:"country_code"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Dist float64 `json:"distance_km"`
		}
		var results []nearby
		for _, a := range airports {
			d := haversineNM(lat, lon, a.Lat, a.Lon)
			if d <= radiusNM {
				results = append(results, nearby{a.ICAO, a.IATA, a.Name, a.Country, a.Lat, a.Lon, math.Round(d*1.852*10) / 10})
			}
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Dist < results[j].Dist })
		if len(results) > limit { results = results[:limit] }
		writeJSON(w, results)
	})

	// === MCP v1 ===

	mcpManifest, _ := json.Marshal(map[string]any{
		"name": "hpr-traffic-api", "version": Version,
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
				Prop: map[string]any{"name": p.Name, "country_code": p.CountryCode, "port_size": p.PortSize, "locode": p.LOCODE, "zone_code": p.ZoneCode, "wpi_id": p.WPIID, "max_vessel_size": p.MaxVesselSize, "channel_depth_m": p.ChannelDepth, "cargo_depth_m": p.CargoDepth},
			})
		}
		writeJSON(w, map[string]any{"type": "FeatureCollection", "features": features})
	})

	// === Airports GeoJSON (for map demo) ===
	mux.HandleFunc("/v1/airports/geojson", func(w http.ResponseWriter, r *http.Request) {
		type feat struct {
			Type string         `json:"type"`
			Geom map[string]any `json:"geometry"`
			Prop map[string]any `json:"properties"`
		}
		features := make([]feat, 0, len(airports))
		for _, a := range airports {
			features = append(features, feat{
				Type: "Feature",
				Geom: map[string]any{"type": "Point", "coordinates": []float64{a.Lon, a.Lat}},
				Prop: map[string]any{"icao": a.ICAO, "iata": a.IATA, "name": a.Name, "city": a.City, "country": a.Country, "route_count": len(byAirport[a.ICAO])},
			})
		}
		writeJSON(w, map[string]any{"type": "FeatureCollection", "features": features})
	})

	// === HPR-Atlas Binary WebSocket (/ws) ===
	// Streams binary frames: ports snapshot on connect, then on-demand queries
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// Upgrade to WebSocket (stdlib, no deps)
		if r.Header.Get("Upgrade") != "websocket" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"websocket upgrade required"}`))
			return
		}
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Sec-WebSocket-Accept", computeAcceptKey(r.Header.Get("Sec-WebSocket-Key")))
		w.Header().Set("Sec-WebSocket-Protocol", "hpra-v1")
		w.WriteHeader(101)
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buf.Flush()

		// Send ports binary snapshot as first frame
		portsBin := buildPortsBinary()
		writeWSFrame(conn, portsBin)

		// Read loop: client can request specific data
		for {
			msg, err := readWSFrame(conn)
			if err != nil {
				break
			}
			if len(msg) == 0 {
				continue
			}
			switch msg[0] {
			case 0x01: // request airports binary
				writeWSFrame(conn, buildAirportsBinary())
			case 0x02: // request lanes (raw geojson bytes)
				writeWSFrame(conn, shippingLanesJSON)
			case 0x09: // ping
				writeWSFrame(conn, []byte{0x0A}) // pong
			}
		}
	})

	mux.HandleFunc("/ws/hub", handleWSHub)

	// === HPR-Atlas Binary Protocol (P6) ===
	// Packs port/airport data into compact binary:
	// Header: "HPRA" (4B) + version (1B) + type (1B) + count (2B) + string_table_offset (4B) + reserved (4B) = 16B
	// Points: lat_i32 (4B) + lon_i32 (4B) + size_u8 (1B) + name_idx (2B) + country_idx (2B) + flags (1B) + teu_u16 (2B) = 16B each
	// String table: count_u16 + [len_u16 + utf8 bytes]...

	mux.HandleFunc("/v1/ports/bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-HPRA-Format", "ports-v1")
		w.Write(buildPortsBinary())
	})

	mux.HandleFunc("/v1/airports/bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-HPRA-Format", "airports-v1")
		w.Write(buildAirportsBinary())
	})

	// Air lanes: great-circle arcs for routes from/to an airport
	mux.HandleFunc("/v1/air-lanes/geojson", func(w http.ResponseWriter, r *http.Request) {
		icao := strings.ToUpper(r.URL.Query().Get("icao"))
		if icao == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"icao parameter required"}`))
			return
		}
		ap, ok := airports[icao]
		if !ok {
			notFound(w, "Airport not found")
			return
		}
		rts := byAirport[icao]
		limit := 200
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v < 1000 {
				limit = v
			}
		}
		type feat struct {
			Type string         `json:"type"`
			Geom map[string]any `json:"geometry"`
			Prop map[string]any `json:"properties"`
		}
		var features []feat
		count := 0
		seen := make(map[string]bool)
		for _, rt := range rts {
			if count >= limit {
				break
			}
			parts := strings.Split(rt.AirportCodes, "-")
			// Find the other end
			var destICAO string
			for _, p := range parts {
				if p != icao {
					destICAO = p
					break
				}
			}
			if destICAO == "" || seen[destICAO] {
				continue
			}
			seen[destICAO] = true
			dest, ok := airports[destICAO]
			if !ok {
				continue
			}
			coords := greatCircleArc(ap.Lat, ap.Lon, dest.Lat, dest.Lon)
			features = append(features, feat{
				Type: "Feature",
				Geom: map[string]any{"type": "LineString", "coordinates": coords},
				Prop: map[string]any{
					"from": icao, "to": destICAO,
					"airline":     rt.AirlineCode,
					"distance_nm": math.Round(haversineNM(ap.Lat, ap.Lon, dest.Lat, dest.Lon)*10) / 10,
				},
			})
			count++
		}
		w.Header().Set("Content-Type", "application/geo+json")
		json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": features, "airport": ap.Name})
	})

	// === Static demo app ===
	sub, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/css/", fileServer)
	mux.Handle("/js/", fileServer)
	mux.Handle("/mapstyles/", fileServer)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		f, _ := staticFiles.ReadFile("static/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(f)
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
