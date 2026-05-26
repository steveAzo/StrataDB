package main

import (
	"flag"
	"log"
	"net/http"

	"stratadb/db"
	"stratadb/server"
)

func main() {
	dir  := flag.String("dir",  "data",  "directory for SSTable and WAL files")
	addr := flag.String("addr", ":6380", "listen address")
	flag.Parse()

	database, err := db.Open(*dir, 4*1024*1024, 4) // 4MB memtable, compact after 4 L0 files
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	log.Printf("stratadb listening on %s (data dir: %s)", *addr, *dir)
	if err := http.ListenAndServe(*addr, server.New(database).Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
