// Command server runs the Operations Control Room API.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rishabh/operation-control-room/internal/api"
	"github.com/rishabh/operation-control-room/internal/db"
	"github.com/rishabh/operation-control-room/internal/weather"
)

func main() {
	dbPath := getenv("DB_PATH", "control_room.db")
	seedCSV := getenv("SEED_CSV", "data/chicago_trips.csv")
	addr := getenv("ADDR", ":8080")

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := db.EnsureSeeded(conn, seedCSV); err != nil {
		log.Fatalf("seed db: %v", err)
	}

	srv := api.New(conn, weather.NewClient())

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
