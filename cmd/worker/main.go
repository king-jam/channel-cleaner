package main

import (
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/king-jam/slacko-botto/queue"

	_ "github.com/heroku/x/hmetrics/onload" // heroku metrics
)

func main() {
	dbString := os.Getenv("DATABASE_URL")
	if dbString == "" {
		log.Fatal("$DATABASE_URL must be set")
	}
	dbURL, err := url.Parse(dbString)
	if err != nil {
		log.Fatal("Invalid Database URL format")
	}

	qc, err := queue.NewQueue(dbURL)
	if err != nil {
		log.Fatal("Unable to initialize the Database")
	}
	defer qc.Close()

	qc.InitWorkerPool(2)

	// Catch signal so we can shutdown gracefully
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go qc.StartWorkers()

	// Wait for a signal
	sig := <-sigCh
	log.Printf("%s Signal received. Shutting down.", sig.String())
}
