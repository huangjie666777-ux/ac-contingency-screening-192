package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("ac contingency screening listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newRouter()))
}
