package kvsrpc_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kvsv1 "github.com/davidaparicio/gokvs/api/kvs/v1"
	"github.com/davidaparicio/gokvs/internal"
	"github.com/davidaparicio/gokvs/internal/kvsrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testToken = "sesame"

// newTestClient spins up an in-process gRPC server over bufconn with the
// same interceptor chain as cmd/grpc (logging, then optional token auth).
func newTestClient(t *testing.T, withAuth bool, transact internal.TransactionLogger) (kvsv1.KVSServiceClient, *internal.Notifier) {
	t.Helper()

	notifier := internal.NewNotifier()

	unary := []grpc.UnaryServerInterceptor{kvsrpc.UnaryLoggingInterceptor()}
	stream := []grpc.StreamServerInterceptor{kvsrpc.StreamLoggingInterceptor()}
	if withAuth {
		unary = append(unary, kvsrpc.UnaryAuthInterceptor(testToken))
		stream = append(stream, kvsrpc.StreamAuthInterceptor(testToken))
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(stream...),
	)
	kvsv1.RegisterKVSServiceServer(srv, kvsrpc.NewServer(notifier, transact))

	lis := bufconn.Listen(1024 * 1024)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("bufconn serve: %v", err)
		}
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return kvsv1.NewKVSServiceClient(conn), notifier
}

func TestSetGetDelete(t *testing.T) {
	client, _ := newTestClient(t, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Set(ctx, &kvsv1.SetRequest{Key: "grpc-test-a", Value: "1"})
	require.NoError(t, err)

	resp, err := client.Get(ctx, &kvsv1.GetRequest{Key: "grpc-test-a"})
	require.NoError(t, err)
	assert.Equal(t, "1", resp.GetValue())

	_, err = client.Delete(ctx, &kvsv1.DeleteRequest{Key: "grpc-test-a"})
	require.NoError(t, err)

	_, err = client.Get(ctx, &kvsv1.GetRequest{Key: "grpc-test-a"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestEmptyKeyRejected(t *testing.T) {
	client, _ := newTestClient(t, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Get(ctx, &kvsv1.GetRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = client.Set(ctx, &kvsv1.SetRequest{Value: "v"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = client.Delete(ctx, &kvsv1.DeleteRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestWatch(t *testing.T) {
	client, notifier := newTestClient(t, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &kvsv1.WatchRequest{Key: "grpc-test-watched"})
	require.NoError(t, err)

	// The subscription is registered inside the server-side Watch handler,
	// which may not have run yet when client.Watch returns.
	require.Eventually(t, func() bool { return notifier.WatcherCount() == 1 },
		2*time.Second, 10*time.Millisecond, "watch subscription never registered")

	_, err = client.Set(ctx, &kvsv1.SetRequest{Key: "grpc-test-watched", Value: "v1"})
	require.NoError(t, err)

	event, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, kvsv1.EventType_EVENT_TYPE_SET, event.GetType())
	assert.Equal(t, "grpc-test-watched", event.GetKey())
	assert.Equal(t, "v1", event.GetValue())

	// Changes to other keys must not reach a key-scoped watcher.
	_, err = client.Set(ctx, &kvsv1.SetRequest{Key: "grpc-test-other", Value: "x"})
	require.NoError(t, err)
	_, err = client.Delete(ctx, &kvsv1.DeleteRequest{Key: "grpc-test-watched"})
	require.NoError(t, err)

	event, err = stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, kvsv1.EventType_EVENT_TYPE_DELETE, event.GetType())
	assert.Equal(t, "grpc-test-watched", event.GetKey())
}

func TestWatchAllKeys(t *testing.T) {
	client, notifier := newTestClient(t, false, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &kvsv1.WatchRequest{}) // empty key = all keys
	require.NoError(t, err)

	require.Eventually(t, func() bool { return notifier.WatcherCount() == 1 },
		2*time.Second, 10*time.Millisecond, "watch subscription never registered")

	_, err = client.Set(ctx, &kvsv1.SetRequest{Key: "grpc-test-any", Value: "v"})
	require.NoError(t, err)

	event, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "grpc-test-any", event.GetKey())
}

func TestAuthInterceptor(t *testing.T) {
	client, notifier := newTestClient(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// No token.
	_, err := client.Get(ctx, &kvsv1.GetRequest{Key: "k"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// Wrong token.
	badCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer wrong")
	_, err = client.Get(badCtx, &kvsv1.GetRequest{Key: "k"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// Malformed header (not a Bearer token).
	badCtx = metadata.AppendToOutgoingContext(ctx, "authorization", testToken)
	_, err = client.Get(badCtx, &kvsv1.GetRequest{Key: "k"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// Valid token: passes auth, then hits the store (NotFound, not Unauthenticated).
	okCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testToken)
	_, err = client.Get(okCtx, &kvsv1.GetRequest{Key: "grpc-test-auth-missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = client.Set(okCtx, &kvsv1.SetRequest{Key: "grpc-test-auth", Value: "v"})
	assert.NoError(t, err)

	// Streams are guarded too: the error surfaces on the first Recv.
	stream, err := client.Watch(ctx, &kvsv1.WatchRequest{})
	require.NoError(t, err)
	_, err = stream.Recv()
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	okStream, err := client.Watch(okCtx, &kvsv1.WatchRequest{Key: "grpc-test-auth-watch"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return notifier.WatcherCount() == 1 },
		2*time.Second, 10*time.Millisecond, "watch subscription never registered")

	_, err = client.Set(okCtx, &kvsv1.SetRequest{Key: "grpc-test-auth-watch", Value: "v"})
	require.NoError(t, err)
	event, err := okStream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "grpc-test-auth-watch", event.GetKey())
}

func TestWatchCancelReleasesSubscription(t *testing.T) {
	client, notifier := newTestClient(t, false, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := client.Watch(ctx, &kvsv1.WatchRequest{Key: "grpc-test-cancel"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return notifier.WatcherCount() == 1 },
		2*time.Second, 10*time.Millisecond, "watch subscription never registered")

	// Cancelling the client context must terminate the server-side handler
	// and release the subscription.
	cancel()
	require.Eventually(t, func() bool { return notifier.WatcherCount() == 0 },
		2*time.Second, 10*time.Millisecond, "watch subscription never released")
}

func TestGatewayAuthorizationHeaderAccepted(t *testing.T) {
	client, _ := newTestClient(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// grpc-gateway forwards the HTTP Authorization header under a
	// "grpcgateway-" prefixed metadata key; the interceptor must accept it.
	gwCtx := metadata.AppendToOutgoingContext(ctx, "grpcgateway-authorization", "Bearer "+testToken)
	_, err := client.Set(gwCtx, &kvsv1.SetRequest{Key: "grpc-test-gw", Value: "v"})
	assert.NoError(t, err)
}

func TestMutationsAppendToTransactionLog(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "transactions.log")
	transact, err := internal.NewTransactionLogger(logFile)
	require.NoError(t, err)
	transact.Run()

	client, _ := newTestClient(t, false, transact)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Set(ctx, &kvsv1.SetRequest{Key: "grpc-test-log", Value: "persisted"})
	require.NoError(t, err)
	_, err = client.Delete(ctx, &kvsv1.DeleteRequest{Key: "grpc-test-log"})
	require.NoError(t, err)

	transact.Wait() // both events flushed to disk
	require.NoError(t, transact.Close())

	data, err := os.ReadFile(logFile) // #nosec G304 -- test-owned temp path
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "grpc-test-log\tpersisted")
	lines := strings.Split(strings.TrimSpace(content), "\n")
	assert.Len(t, lines, 2, "expected one PUT and one DELETE entry")
}
