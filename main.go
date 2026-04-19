package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type Route struct {
	Callsign     string `json:"callsign"`
	Code         string `json:"code"`
	Number       string `json:"number"`
	AirlineCode  string `json:"airline_code"`
	AirportCodes string `json:"airport_codes"`
}

var routes map[string]Route

func loadRoutes(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Read() // skip header
	routes = make(map[string]Route, 500000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		routes[rec[0]] = Route{rec[0], rec[1], rec[2], rec[3], rec[4]}
	}
	return nil
}

func main() {
	if err := loadRoutes("routes.csv"); err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d routes\n", len(routes))

	http.HandleFunc("/v1/routes/", func(w http.ResponseWriter, r *http.Request) {
		callsign := r.URL.Path[len("/v1/routes/"):]
		w.Header().Set("Content-Type", "application/json")
		if route, ok := routes[callsign]; ok {
			json.NewEncoder(w).Encode(route)
		} else {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"Route not found"}`))
		}
	})

	fmt.Println("Listening on :8081")
	http.ListenAndServe(":8081", nil)
}
