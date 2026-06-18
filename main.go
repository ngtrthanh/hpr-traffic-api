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
	TEUThousands       int     `json:"teu_thousands,omitempty"`
	CountryCode        string  `json:"country_code,omitempty"`
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
		var teu int
		if len(rec) > 21 && rec[21] != "" {
			teu, _ = strconv.Atoi(rec[21])
		}
		var cc string
		if len(rec) > 22 {
			cc = rec[22]
		}
		sp := Seaport{
			WPIID: rec[0], Name: rec[1], Country: rec[2], State: rec[3],
			Lat: lat, Lon: lon,
			PortSize: rec[6], MaxVesselSize: rec[7],
			ChannelDepth: chD, CargoDepth: caD, AnchorageDepth: anD, OilTerminalDepth: oiD,
			TidalRange: tid, EntranceRestriction: rec[13],
			LOCODE: rec[14], ZoneCode: rec[15],
			VesselCountTotal: vt, VesselCountContainer: vc, VesselCountDryBulk: vb, VesselCountTanker: vk,
			IndustryTop1: rec[20], TEUThousands: teu, CountryCode: cc,
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
	marnetJSON = data
	var gj struct {
		Features []struct {
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &gj); err != nil {
		return err
	}

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
		// Store polyline coords
		poly := make([][2]float64, len(coords))
		for i, c := range coords {
			poly[i] = [2]float64{c[0], c[1]}
		}
		polyRev := make([][2]float64, len(coords))
		for i, c := range poly {
			polyRev[len(poly)-1-i] = c
		}
		marnetAdj[a] = append(marnetAdj[a], graphEdge{to: b, dist: dist, coords: poly})
		marnetAdj[b] = append(marnetAdj[b], graphEdge{to: a, dist: dist, coords: polyRev})
	}

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
		pts[i] = []float64{lon, lat}
	}
	return pts
}

// Find nearest graph node to given lon,lat within search radius
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
func dijkstraRoute(from, to nodeID) ([][2]float64, float64) {
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
		return nil, 0
	}

	// Reconstruct path
	var path [][2]float64
	for cur := to; cur != from; cur = prev[cur] {
		edge := marnetAdj[prev[cur]][prevEdge[cur]]
		// edge.coords goes from prev[cur] → cur, append in reverse order (skip last to avoid dup)
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
	return path, dist[to]
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
		cs := p.Country
		if p.CountryCode != "" {
			cs = p.Country + "|" + p.CountryCode
		}
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
		pts = append(pts, ba{int32(a.Lat * 1e6), int32(a.Lon * 1e6), addStr(a.Name), addStr(a.ICAO), addStr(a.IATA), uint16(len(byAirport[a.ICAO]))})
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
		{"aircraft", loadAircraft, "aircraft.csv"},
		{"airlines", loadAirlines, "airlines.csv"},
		{"airports", loadAirports, "airports.csv"},
		{"routes", loadRoutes, "routes.csv"},
		{"seaports", loadSeaports, "seaports.csv"},
		{"sea_routes", loadSeaRoutes, "sea_distances.csv"},
		{"sea_distance_ports", loadSeaDistancePorts, "sea_distance_ports.csv"},
		{"shipping_lanes", loadShippingLanes, "shipping_lanes.geojson"},
		{"marnet", loadMarnet, "marnet.geojson"},
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
		w.Write(shippingLanesJSON)
	})

	// Marnet network as lanes visualization (alternative to CIA data)
	mux.HandleFunc("/v1/shipping-lanes/marnet", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(marnetJSON)
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
		path, dist := dijkstraRoute(srcNode, dstNode)
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
