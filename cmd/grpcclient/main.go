// Command grpcclient is a small CLI for the gokvs gRPC server.
//
// Usage:
//
//	grpcclient [flags] get <key>
//	grpcclient [flags] set <key> <value>
//	grpcclient [flags] del <key>
//	grpcclient [flags] watch [key]   (empty key watches everything, Ctrl-C to stop)
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	kvsv1 "github.com/davidaparicio/gokvs/api/kvs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	var (
		addr   = flag.String("addr", "localhost:50051", "gRPC server address")
		token  = flag.String("token", os.Getenv("KVS_AUTH_TOKEN"), "bearer token sent with every RPC")
		caCert = flag.String("ca-cert", "", "trusted CA/server certificate (PEM); enables TLS")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	creds := insecure.NewCredentials()
	if *caCert != "" {
		pem, err := os.ReadFile(*caCert) // #nosec G304 -- operator-supplied path
		if err != nil {
			log.Fatalf("reading CA cert: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			log.Fatalf("no certificates found in %s", *caCert)
		}
		creds = credentials.NewTLS(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	}

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("connecting to %s: %v", *addr, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("closing connection: %v", err)
		}
	}()

	client := kvsv1.NewKVSServiceClient(conn)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*token)
	}

	if err := run(ctx, client, args); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, client kvsv1.KVSServiceClient, args []string) error {
	switch cmd := args[0]; cmd {
	case "get":
		if len(args) != 2 {
			return errors.New("usage: get <key>")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		resp, err := client.Get(ctx, &kvsv1.GetRequest{Key: args[1]})
		if err != nil {
			return err
		}
		fmt.Println(resp.GetValue())

	case "set":
		if len(args) != 3 {
			return errors.New("usage: set <key> <value>")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := client.Set(ctx, &kvsv1.SetRequest{Key: args[1], Value: args[2]}); err != nil {
			return err
		}
		fmt.Println("OK")

	case "del", "delete":
		if len(args) != 2 {
			return errors.New("usage: del <key>")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := client.Delete(ctx, &kvsv1.DeleteRequest{Key: args[1]}); err != nil {
			return err
		}
		fmt.Println("OK")

	case "watch":
		key := ""
		if len(args) > 1 {
			key = args[1]
		}
		stream, err := client.Watch(ctx, &kvsv1.WatchRequest{Key: key})
		if err != nil {
			return err
		}
		log.Printf("watching %q (empty = all keys), Ctrl-C to stop", key)
		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) || status.Code(err) == codes.Canceled {
				return nil
			}
			if err != nil {
				return err
			}
			fmt.Printf("%s key=%s value=%q\n", event.GetType(), event.GetKey(), event.GetValue())
		}

	default:
		return fmt.Errorf("unknown command %q (want get|set|del|watch)", cmd)
	}
	return nil
}
