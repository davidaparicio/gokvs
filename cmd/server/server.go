package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/davidaparicio/gokvs/internal"
	"github.com/gorilla/mux"
)

var transact *internal.TransactionLogger

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
	})
}

func notAllowedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Allowed", http.StatusMethodNotAllowed)
}

func keyValuePutHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	value, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = internal.Put(key, string(value))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	transact.WritePut(key, string(value))

	log.Printf("PUT key=%s value=%s\n", key, string(value))
}

func keyValueGetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	value, err := internal.Get(key)
	if errors.Is(err, internal.ErrorNoSuchKey) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write([]byte(value)); err != nil {
		log.Printf("ERROR in w.Write for GET key=%s\n", key)
	}

	log.Printf("GET key=%s\n", key)
}

func keyValueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	err := internal.Delete(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	transact.WriteDelete(key)

	log.Printf("DELETE key=%s\n", key)
}

func checkMuxHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("imok\n")); err != nil {
		log.Printf("ERROR in w.Write for ruok\n")
	}
}

func initializeTransactionLog() error {
	var err error

	transact, err = internal.NewTransactionLogger("/tmp/transactions.log")
	if err != nil {
		return fmt.Errorf("failed to create transaction logger: %w", err)
	}

	events, errors := transact.ReadEvents()
	count, ok, e := 0, true, internal.Event{}

	for ok && err == nil {
		select {
		case err, ok = <-errors:

		case e, ok = <-events:
			switch e.EventType {
			case internal.EventDelete: // Got a DELETE event!
				err = internal.Delete(e.Key)
				count++
			case internal.EventPut: // Got a PUT event!
				err = internal.Put(e.Key, e.Value)
				count++
			}
		}
	}

	log.Printf("%d events replayed\n", count)

	transact.Run()

	return err
}

func main() {
	// Initializes the transaction log and loads existing data, if any.
	// Blocks until all data is read.
	err := initializeTransactionLog()
	if err != nil {
		panic(err)
	}

	// Create a new mux router
	r := mux.NewRouter()

	r.Use(loggingMiddleware)

	// Associate a path with a handler function on the router
	r.HandleFunc("/v1/{key}", keyValueGetHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", keyValuePutHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", keyValueDeleteHandler).Methods("DELETE")

	r.HandleFunc("/healthz", checkMuxHandler)
	r.HandleFunc("/ruok", checkMuxHandler)

	r.HandleFunc("/", notAllowedHandler)
	r.HandleFunc("/v1", notAllowedHandler)
	r.HandleFunc("/v1/{key}", notAllowedHandler)

	srv := &http.Server{
		Addr:              ":8080",
		ReadTimeout:       1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		Handler:           r,
		//TLSConfig:       tlsConfig,
	}

	log.Println("Server running on port 8080")
	// Bind to a port and pass in the mux router
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
