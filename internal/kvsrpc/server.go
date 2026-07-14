// Package kvsrpc implements the kvs.v1.KVSService gRPC API on top of the
// in-memory store from the internal package.
package kvsrpc

import (
	"context"
	"errors"

	kvsv1 "github.com/davidaparicio/gokvs/api/kvs/v1"
	"github.com/davidaparicio/gokvs/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements kvsv1.KVSServiceServer.
type Server struct {
	kvsv1.UnimplementedKVSServiceServer

	notifier *internal.Notifier
	transact internal.TransactionLogger // may be nil (no persistence)
}

// NewServer returns a Server publishing change events to notifier and,
// when transact is non-nil, appending mutations to the transaction log.
func NewServer(notifier *internal.Notifier, transact internal.TransactionLogger) *Server {
	return &Server{notifier: notifier, transact: transact}
}

func (s *Server) Get(_ context.Context, req *kvsv1.GetRequest) (*kvsv1.GetResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key must not be empty")
	}

	value, err := internal.Get(req.GetKey())
	if errors.Is(err, internal.ErrorNoSuchKey) {
		return nil, status.Errorf(codes.NotFound, "no such key %q", req.GetKey())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &kvsv1.GetResponse{Value: value}, nil
}

func (s *Server) Set(_ context.Context, req *kvsv1.SetRequest) (*kvsv1.SetResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key must not be empty")
	}

	if err := internal.Put(req.GetKey(), req.GetValue()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if s.transact != nil {
		s.transact.WritePut(req.GetKey(), req.GetValue())
	}
	s.notifier.Publish(internal.NotifyEvent{
		Type:  internal.EventPut,
		Key:   req.GetKey(),
		Value: req.GetValue(),
	})

	return &kvsv1.SetResponse{}, nil
}

func (s *Server) Delete(_ context.Context, req *kvsv1.DeleteRequest) (*kvsv1.DeleteResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key must not be empty")
	}

	if err := internal.Delete(req.GetKey()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if s.transact != nil {
		s.transact.WriteDelete(req.GetKey())
	}
	s.notifier.Publish(internal.NotifyEvent{
		Type: internal.EventDelete,
		Key:  req.GetKey(),
	})

	return &kvsv1.DeleteResponse{}, nil
}

func (s *Server) Watch(req *kvsv1.WatchRequest, stream kvsv1.KVSService_WatchServer) error {
	events, cancel := s.notifier.Subscribe(req.GetKey())
	defer cancel()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		case e := <-events:
			resp := &kvsv1.WatchResponse{Key: e.Key, Value: e.Value}
			switch e.Type {
			case internal.EventPut:
				resp.Type = kvsv1.EventType_EVENT_TYPE_SET
			case internal.EventDelete:
				resp.Type = kvsv1.EventType_EVENT_TYPE_DELETE
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}
