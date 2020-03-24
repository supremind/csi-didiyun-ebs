package dataset

import (
	"context"
	"strings"
	"sync"
	"time"

	ava_grpc "git.supremind.info/products/atom/com/grpc"
	"git.supremind.info/products/atom/proto/go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type dsClientInterface interface {
	datasetService(endpoint, user string) (api.DatasetServiceClient, error)
	volumeService(endpoint, user string) (api.VolumeServiceClient, error)
	close()
}

type dsClient struct {
	lock    sync.Mutex
	clients map[string]*grpc.ClientConn
	timeout time.Duration
}

func newDsClient(to time.Duration) *dsClient {
	return &dsClient{
		clients: make(map[string]*grpc.ClientConn),
		timeout: to,
	}
}

func (t *dsClient) close() {
	t.lock.Lock()
	// close all conns
	for _, c := range t.clients {
		c.Close()
	}
	t.lock.Unlock()
}

func (t *dsClient) datasetService(endpoint, user string) (api.DatasetServiceClient, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	conn := t.clients[user+"@"+endpoint]
	if conn == nil {
		var e error
		conn, e = t.getAuthConn(endpoint, user)
		if e != nil {
			return nil, e
		}
		t.clients[user+"@"+endpoint] = conn
	}

	return api.NewDatasetServiceClient(conn), nil
}

func (t *dsClient) volumeService(endpoint, user string) (api.VolumeServiceClient, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	conn := t.clients[user+"@"+endpoint]
	if conn == nil {
		var e error
		conn, e = t.getAuthConn(endpoint, user)
		if e != nil {
			return nil, e
		}
		t.clients[user+"@"+endpoint] = conn
	}

	return api.NewVolumeServiceClient(conn), nil
}

func (t *dsClient) getAuthConn(endpoint, user string) (*grpc.ClientConn, error) {
	name := user
	index := strings.Index(user, "@")
	if index != -1 {
		name = user[:index]
	}
	authu := func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		c := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{
			ava_grpc.EmailKey:    user,
			ava_grpc.UsernameKey: name,
		}))
		return invoker(c, method, req, reply, cc, opts...)
	}
	auths := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		c := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{
			ava_grpc.EmailKey:    user,
			ava_grpc.UsernameKey: name,
		}))
		return streamer(c, desc, cc, method, opts...)
	}

	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(authu),
		grpc.WithStreamInterceptor(auths),
	}

	if t.timeout > 0 {
		opts = append(opts, grpc.WithTimeout(t.timeout))
	}

	conn, e := grpc.Dial(endpoint, opts...)
	if e != nil {
		return nil, e
	}
	return conn, nil
}
