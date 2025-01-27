package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/davidaparicio/gokvs/internal"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var transact *internal.TransactionLog
var m *internal.Metrics

// prometheusMiddleware implements mux.MiddlewareFunc + loggingMiddleware
func prometheusLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.RequestURI)
		//route := mux.CurrentRoute(r); path, _ := route.GetPathTemplate()
		timer := prometheus.NewTimer(m.RequestDurationHistogram.WithLabelValues(r.Method, r.RequestURI))
		next.ServeHTTP(w, r)
		timer.ObserveDuration()
	})
}

func notAllowedHandler(w http.ResponseWriter, r *http.Request) {
	m.HttpNotAllowed.Inc()
	http.Error(w, "Not Allowed", http.StatusMethodNotAllowed)
}

func keyValuePutHandler(w http.ResponseWriter, r *http.Request) {
	m.QueriesInflight.Inc()
	defer m.QueriesInflight.Dec()
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

	m.EventsPut.Inc()
	log.Printf("PUT key=%s value=%s\n", key, string(value))
}

func keyValueGetHandler(w http.ResponseWriter, r *http.Request) {
	m.QueriesInflight.Inc()
	defer m.QueriesInflight.Dec()
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

	m.EventsGet.Inc()
	log.Printf("GET key=%s\n", key)
}

func keyValueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	m.QueriesInflight.Inc()
	defer m.QueriesInflight.Dec()
	vars := mux.Vars(r)
	key := vars["key"]

	err := internal.Delete(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	transact.WriteDelete(key)

	m.EventsDelete.Inc()
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
			case internal.EventPut: // Got a PUT event!
				err = internal.Put(e.Key, e.Value)
			}
			m.EventsReplayed.Inc()
			count++
		}
	}
	log.Printf("%d events replayed\n", count)

	transact.Run()

	return err
}

func main() {
	internal.PrintVersion()

	// Create a non-global registry.
	reg := prometheus.NewRegistry()
	// Keep all the golang default metrics
	reg.MustRegister(collectors.NewGoCollector())
	// Create new metrics and register them using the custom registry.
	m = internal.NewMetrics(reg)
	m.Info.With(prometheus.Labels{"version": internal.Version}).Set(1)

	// Initializes the transaction log and loads existing data, if any.
	// Blocks until all data is read.
	err := initializeTransactionLog()
	if err != nil {
		panic(err)
	}

	// Create a new mux router
	r := mux.NewRouter()

	r.Use(prometheusLoggingMiddleware)

	// Associate a path with a handler function on the router
	r.HandleFunc("/v1/{key}", keyValueGetHandler).Methods("GET")
	r.HandleFunc("/v1/{key}", keyValuePutHandler).Methods("PUT")
	r.HandleFunc("/v1/{key}", keyValueDeleteHandler).Methods("DELETE")

	r.HandleFunc("/healthz", checkMuxHandler)
	r.HandleFunc("/ruok", checkMuxHandler)

	// Expose metrics and custom registry via an HTTP server
	// using the HandleFor function. "/metrics" is the usual endpoint for that.
	r.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	r.HandleFunc("/", notAllowedHandler)
	r.HandleFunc("/v1", notAllowedHandler)
	r.HandleFunc("/v1/{key}", notAllowedHandler)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// srv := &http.Server{
	// 	Addr:              ":8080",
	// 	ReadTimeout:       1 * time.Second,
	// 	WriteTimeout:      1 * time.Second,
	// 	IdleTimeout:       30 * time.Second,
	// 	ReadHeaderTimeout: 2 * time.Second,
	// 	Handler:           r,
	// 	//TLSConfig: tlsConfig,
	// }

	// Improvement possible https://pkg.go.dev/golang.org/x/sync/errgroup
	// https://www.rudderstack.com/blog/implementing-graceful-shutdown-in-go/
	var wg sync.WaitGroup
	wg.Add(1)

	// Check for a closing signal
	go func() {
		// Graceful shutdown goroutine
		sigquit := make(chan os.Signal, 1)
		// os.Kill can't be caught https://groups.google.com/g/golang-nuts/c/t2u-RkKbJdU
		// POSIX spec: signal can be caught except SIGKILL/SIGSTOP signals
		// Ctrl-c (usually) sends the SIGINT signal, not SIGKILL
		// syscall.SIGTERM usual signal for termination
		// and default one for docker containers, which is also used by kubernetes
		signal.Notify(sigquit, os.Interrupt, syscall.SIGTERM)
		sig := <-sigquit

		log.Println() // newline "\r\n" to let the signal alone, like ^C
		log.Printf("Caught the following signal: %+v", sig)

		log.Printf("Gracefully shutting down server..")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Unable to shutdown server: %v", err)
		} else {
			log.Printf("Server stopped")
		}

		log.Printf("Gracefully shutting down TransactionLogger...")
		if err := transact.Close(); err != nil {
			log.Printf("Unable to close FileTransactionLogger: %v", err)
		} else {
			log.Printf("FileTransactionLogger closed")
		}

		wg.Done()
	}()

	log.Println("Server running on port 8080")
	// Bind to a port and pass in the mux router
	if err := srv.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			log.Printf("Server stopping...")
		} else {
			log.Fatal(err) //TODO replace Fatal by a graceful shutdown
		}
	}
	wg.Wait() //For the signal/graceful shutdown goroutine
}
