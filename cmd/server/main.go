package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/huangjie666777-ux/ac-contingency-screening-192/internal/api"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	readTimeout := flag.Duration("read-timeout", 15*time.Second, "maximum HTTP read duration")
	writeTimeout := flag.Duration("write-timeout", 30*time.Second, "maximum HTTP write duration")
	flag.Parse()

	server := &http.Server{
		Addr:         *addr,
		Handler:      api.NewServer().Handler(),
		ReadTimeout:  *readTimeout,
		WriteTimeout: *writeTimeout,
	}
	log.Printf("N-1 AC contingency screening API listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
