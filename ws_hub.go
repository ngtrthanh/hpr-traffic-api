package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type hubParams map[string]json.RawMessage

type hubSession struct {
	conn net.Conn
	mu   sync.Mutex
}

func (s *hubSession) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeWSTextFrame(s.conn, b)
}

// writeWSTextFrame writes a WebSocket text frame (opcode 0x01, FIN=1).
func writeWSTextFrame(conn net.Conn, payload []byte) {
	hdr := []byte{0x81}
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

// handleWSHub is the authenticated JSON WebSocket endpoint consumed by hpr-atlas.
//
// Auth: WS_HUB_TOKEN env — pass via X-Hub-Token header or ?token= query param.
// On connect → server pushes {type:"push",topic:"ports",data:[...]} and notable_ships.
// Client sends {type:"req",id:"<id>",tool:"<tool>",params:{...}}
// Server replies {type:"res",id:"<id>",data:{...}} or {type:"err",id:"<id>",error:"..."}
func handleWSHub(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("WS_HUB_TOKEN")
	if token == "" {
		w.WriteHeader(503)
		w.Write([]byte(`{"error":"hub not configured"}`))
		return
	}

	var provided string
	if v := r.Header.Get("X-Hub-Token"); v != "" {
		provided = v
	} else {
		provided = r.URL.Query().Get("token")
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}

	if r.Header.Get("Upgrade") != "websocket" {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"websocket upgrade required"}`))
		return
	}

	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Sec-WebSocket-Accept", computeAcceptKey(r.Header.Get("Sec-WebSocket-Key")))
	w.Header().Set("Sec-WebSocket-Protocol", "hpra-hub-v1")
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

	sess := &hubSession{conn: conn}

	hubPushPorts(sess)
	hubPushNotableShips(sess)

	pingStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sess.mu.Lock()
				conn.Write([]byte{0x89, 0x00}) // ping frame
				sess.mu.Unlock()
			case <-pingStop:
				return
			}
		}
	}()
	defer close(pingStop)

	for {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		raw, err := readWSFrame(conn)
		if err != nil {
			break
		}
		if len(raw) == 0 {
			continue
		}
		var msg struct {
			Type   string    `json:"type"`
			ID     string    `json:"id"`
			Tool   string    `json:"tool"`
			Params hubParams `json:"params"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Type != "req" {
			continue
		}
		hubDispatch(sess, msg.ID, msg.Tool, msg.Params)
	}
	log.Printf("ws/hub: client disconnected")
}

func hubDispatch(sess *hubSession, id, tool string, params hubParams) {
	replyErr := func(msg string) {
		sess.send(map[string]any{"type": "err", "id": id, "error": msg})
	}
	replyData := func(data any) {
		b, _ := json.Marshal(data)
		sess.send(map[string]any{"type": "res", "id": id, "data": json.RawMessage(b)})
	}

	switch tool {
	case "lookup_ships":
		var mmsis []string
		if err := hubDecodeParam(params, "mmsis", &mmsis); err != nil || len(mmsis) == 0 {
			replyErr("mmsis required")
			return
		}
		if len(mmsis) > 100 {
			mmsis = mmsis[:100]
		}
		result := make(map[string]*Ship, len(mmsis))
		for _, m := range mmsis {
			if s := warmLookupShip(m); s != nil {
				result[m] = s
			}
		}
		replyData(result)

	case "lookup_port":
		var portID string
		if err := hubDecodeParam(params, "id", &portID); err != nil || portID == "" {
			replyErr("id required")
			return
		}
		p := hubResolvePort(portID)
		if p == nil {
			replyErr("port not found: " + portID)
			return
		}
		replyData(p)

	case "nearby_ports":
		var lat, lon, radiusKM float64
		var limit int
		hubDecodeParam(params, "lat", &lat)
		hubDecodeParam(params, "lon", &lon)
		hubDecodeParam(params, "radius_km", &radiusKM)
		hubDecodeParam(params, "limit", &limit)
		if radiusKM <= 0 {
			radiusKM = 50
		}
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		radiusNM := radiusKM / 1.852
		type nearby struct {
			*Seaport
			DistanceNM float64 `json:"distance_nm"`
		}
		var results []nearby
		for i := range seaports {
			d := haversineNM(lat, lon, seaports[i].Lat, seaports[i].Lon)
			if d <= radiusNM {
				results = append(results, nearby{&seaports[i], math.Round(d*10) / 10})
			}
		}
		for i := 1; i < len(results); i++ {
			for j := i; j > 0 && results[j].DistanceNM < results[j-1].DistanceNM; j-- {
				results[j], results[j-1] = results[j-1], results[j]
			}
		}
		if len(results) > limit {
			results = results[:limit]
		}
		replyData(results)

	case "sea_route":
		var fromS, toS string
		hubDecodeParam(params, "from", &fromS)
		hubDecodeParam(params, "to", &toS)
		fromLat, fromLon, ok := parseLatLon(fromS)
		if !ok {
			replyErr("from must be 'lat,lon'")
			return
		}
		var toLat, toLon float64
		var toName string
		if tLat, tLon, ok2 := parseLatLon(toS); ok2 {
			toLat, toLon = tLat, tLon
		} else {
			p := hubResolvePort(toS)
			if p == nil {
				replyErr("to: port not found: " + toS)
				return
			}
			toLat, toLon, toName = p.Lat, p.Lon, p.Name
		}
		srcNode, _ := nearestNode(fromLon, fromLat)
		dstNode, _ := nearestNode(toLon, toLat)
		if srcNode < 0 || dstNode < 0 {
			replyErr("position too far from maritime network")
			return
		}
		path, distNM, passes := dijkstraRoute(srcNode, dstNode)
		if path == nil {
			replyErr("no route found")
			return
		}
		coords := make([][]float64, len(path))
		for i, p := range path {
			coords[i] = []float64{p[0], p[1]}
		}
		replyData(map[string]any{
			"coordinates":      coords,
			"distance_nm":      math.Round(distNM*10) / 10,
			"distance_km":      math.Round(distNM*1.852*10) / 10,
			"passes":           passes,
			"destination_name": toName,
		})

	case "voyage_eta":
		var lat, lon, speedKn float64
		var toS string
		hubDecodeParam(params, "lat", &lat)
		hubDecodeParam(params, "lon", &lon)
		hubDecodeParam(params, "to", &toS)
		hubDecodeParam(params, "speed_kn", &speedKn)
		if speedKn <= 0 {
			speedKn = 14
		}
		toPort := hubResolvePort(toS)
		if toPort == nil {
			replyErr("port not found: " + toS)
			return
		}
		srcNode, _ := nearestNode(lon, lat)
		dstNode, _ := nearestNode(toPort.Lon, toPort.Lat)
		if srcNode < 0 || dstNode < 0 {
			replyErr("position too far from maritime network")
			return
		}
		path, remainNM, passes := dijkstraRoute(srcNode, dstNode)
		if path == nil {
			replyErr("no route found")
			return
		}
		etaHours := remainNM / speedKn
		eta := time.Now().Add(time.Duration(etaHours * float64(time.Hour)))
		replyData(map[string]any{
			"destination":      map[string]any{"name": toPort.Name, "locode": toPort.LOCODE, "country_code": toPort.CountryCode},
			"remaining_nm":     math.Round(remainNM*10) / 10,
			"remaining_km":     math.Round(remainNM*1.852*10) / 10,
			"speed_kn":         speedKn,
			"eta_hours":        math.Round(etaHours*10) / 10,
			"eta_utc":          eta.UTC().Format(time.RFC3339),
			"passes_remaining": passes,
			"waypoints":        len(path),
		})

	case "notable_ships":
		hubPushNotableShips(sess)

	default:
		replyErr("unknown tool: " + tool)
	}
}

func hubPushPorts(sess *hubSession) {
	b, _ := json.Marshal(seaports)
	sess.send(map[string]any{
		"type":  "push",
		"topic": "ports",
		"data":  json.RawMessage(b),
	})
}

func hubPushNotableShips(sess *hubSession) {
	b, _ := json.Marshal(notableShips)
	sess.send(map[string]any{
		"type":  "push",
		"topic": "notable_ships",
		"data":  json.RawMessage(b),
	})
}

// hubResolvePort resolves a port by LOCODE, WPI ID, or name (exact then fuzzy).
func hubResolvePort(q string) *Seaport {
	q = strings.TrimSpace(q)
	qu := strings.ToUpper(q)
	qn := strings.ReplaceAll(strings.ReplaceAll(qu, " ", ""), "-", "")
	if p, ok := portByLOCODE[qu]; ok {
		return p
	}
	for i := range seaports {
		if strings.ToUpper(seaports[i].LOCODE) == qu {
			return &seaports[i]
		}
	}
	if p, ok := portByWPI[q]; ok {
		return p
	}
	for i := range seaports {
		if strings.ToUpper(seaports[i].Name) == qu {
			return &seaports[i]
		}
	}
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

func hubDecodeParam(params hubParams, key string, dst any) error {
	v, ok := params[key]
	if !ok {
		return nil
	}
	return json.Unmarshal(v, dst)
}

// parseLatLon parses "lat,lon" into float64 components.
func parseLatLon(s string) (lat, lon float64, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ",", 2)
	if len(parts) != 2 {
		return
	}
	var err error
	if lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err != nil {
		return
	}
	if lon, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err != nil {
		return
	}
	ok = true
	return
}
