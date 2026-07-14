// Command grpc serves the key-value store over gRPC (unary Get/Set/Delete
// plus a server-streaming Watch), with logging and token-auth interceptors,
// optional TLS, and an optional REST layer via grpc-gateway.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kvsv1 "github.com/davidaparicio/gokvs/api/kvs/v1"
	"github.com/davidaparicio/gokvs/internal"
	"github.com/davidaparicio/gokvs/internal/kvsrpc"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var (
		grpcAddr  = flag.String("grpc-addr", ":50051", "gRPC listen address")
		httpAddr  = flag.String("http-addr", ":8081", "REST gateway listen address (empty to disable)")
		authToken = flag.String("auth-token", os.Getenv("KVS_AUTH_TOKEN"), "bearer token required on every RPC (empty to disable auth)")
		tlsCert   = flag.String("tls-cert", "", "path to the TLS certificate (PEM); TLS is enabled when both -tls-cert and -tls-key are set")
		tlsKey    = flag.String("tls-key", "", "path to the TLS private key (PEM)")
		logFile   = flag.String("transactions", "/tmp/transactions-grpc.log", "transaction log file (empty to disable persistence)")
	)
	flag.Parse()

	internal.PrintVersion()

	transact, err := initializeTransactionLog(*logFile)
	if err != nil {
		log.Fatalf("transaction log: %v", err)
	}

	// Interceptor chain: logging first, then token auth.
	unary := []grpc.UnaryServerInterceptor{kvsrpc.UnaryLoggingInterceptor()}
	stream := []grpc.StreamServerInterceptor{kvsrpc.StreamLoggingInterceptor()}
	if *authToken != "" {
		unary = append(unary, kvsrpc.UnaryAuthInterceptor(*authToken))
		stream = append(stream, kvsrpc.StreamAuthInterceptor(*authToken))
	} else {
		log.Println("WARNING: no auth token configured, all calls are accepted")
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(stream...),
	}

	useTLS := *tlsCert != "" && *tlsKey != ""
	if useTLS {
		creds, err := credentials.NewServerTLSFromFile(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("failed to load TLS credentials: %v", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)
	kvsv1.RegisterKVSServiceServer(grpcServer, kvsrpc.NewServer(internal.NewNotifier(), transactLogger(transact)))

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *grpcAddr, err)
	}

	// Optional REST layer: grpc-gateway translates HTTP+JSON to gRPC by
	// dialing back into this same server over loopback.
	var httpServer *http.Server
	if *httpAddr != "" {
		httpServer, err = newGateway(*httpAddr, *grpcAddr, *tlsCert, useTLS)
		if err != nil {
			log.Fatalf("failed to set up REST gateway: %v", err)
		}
		go func() {
			log.Printf("REST gateway listening on %s", *httpAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("gateway: %v", err)
			}
		}()
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sigquit := make(chan os.Signal, 1)
		signal.Notify(sigquit, os.Interrupt, syscall.SIGTERM)
		sig := <-sigquit
		log.Printf("caught signal %v, shutting down...", sig)

		if httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(ctx); err != nil {
				log.Printf("gateway shutdown: %v", err)
			}
		}
		grpcServer.GracefulStop()

		if transact != nil {
			if err := transact.Close(); err != nil {
				log.Printf("transaction log close: %v", err)
			}
		}
	}()

	log.Printf("gRPC server listening on %s (TLS: %v)", *grpcAddr, useTLS)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC serve: %v", err)
	}
	log.Println("server stopped")
}

// transactLogger converts a possibly-nil *TransactionLog into the interface
// without wrapping a nil pointer in a non-nil interface value.
func transactLogger(t *internal.TransactionLog) internal.TransactionLogger {
	if t == nil {
		return nil
	}
	return t
}

// initializeTransactionLog opens the log at path, replays stored events into
// the in-memory store, and starts the log writer. An empty path disables
// persistence.
func initializeTransactionLog(path string) (*internal.TransactionLog, error) {
	if path == "" {
		return nil, nil
	}

	transact, err := internal.NewTransactionLogger(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction logger: %w", err)
	}

	events, errs := transact.ReadEvents()
	count, ok, e := 0, true, internal.Event{}

	for ok && err == nil {
		select {
		case err, ok = <-errs:

		case e, ok = <-events:
			switch e.EventType {
			case internal.EventDelete:
				err = internal.Delete(e.Key)
			case internal.EventPut:
				err = internal.Put(e.Key, e.Value)
			}
			if ok {
				count++
			}
		}
	}
	log.Printf("%d events replayed", count)

	transact.Run()

	return transact, err
}

// newGateway builds the grpc-gateway HTTP server proxying to grpcAddr.
// When the gRPC server runs with TLS, the gateway dials it using tlsCert as
// the trusted root CA (works with the self-signed certs from scripts/gen-certs.sh).
func newGateway(httpAddr, grpcAddr, tlsCert string, useTLS bool) (*http.Server, error) {
	var dialCreds credentials.TransportCredentials
	if useTLS {
		pem, err := os.ReadFile(tlsCert) // #nosec G304 -- operator-supplied path
		if err != nil {
			return nil, fmt.Errorf("reading TLS cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", tlsCert)
		}
		dialCreds = credentials.NewTLS(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	} else {
		dialCreds = insecure.NewCredentials()
	}

	// A listen address like ":50051" has no host to dial; use localhost.
	dialTarget := grpcAddr
	if strings.HasPrefix(dialTarget, ":") {
		dialTarget = "localhost" + dialTarget
	}

	conn, err := grpc.NewClient(dialTarget, grpc.WithTransportCredentials(dialCreds))
	if err != nil {
		return nil, fmt.Errorf("dialing gRPC server: %w", err)
	}

	mux := runtime.NewServeMux()
	if err := kvsv1.RegisterKVSServiceHandler(context.Background(), mux, conn); err != nil {
		return nil, fmt.Errorf("registering gateway handler: %w", err)
	}

	return &http.Server{
		Addr:              httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       30 * time.Second,
	}, nil
}
