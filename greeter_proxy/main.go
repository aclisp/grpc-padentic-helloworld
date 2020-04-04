package main

import (
	"context"
	"github.com/vgough/grpc-proxy/connector"
	"github.com/vgough/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"grpc-padentic-helloworld/registry"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	serviceName = "com.github.aclisp.grpcpadentic.proxy"
	address     = "127.0.0.1:0"
)

type director struct {
	cr *connector.CachingConnector
}

type serviceNameKey struct{}

type proxyServer struct {
	listener net.Listener
	stop     chan struct{}
}

func newProxy(l net.Listener) (s *proxyServer) {
	return &proxyServer{
		listener: l,
		stop:     make(chan struct{}, 1),
	}
}

func (d *director) Connect(ctx context.Context, method string) (context.Context, *grpc.ClientConn, error) {
	log.Printf("director connect %q", method)
	fullServiceName := strings.SplitN(method, "/", 3)[1]
	serviceName := fullServiceName[:strings.LastIndexByte(fullServiceName, '.')]
	conn, err := d.cr.Dial(ctx, serviceName)
	newCtx := context.WithValue(ctx, serviceNameKey{}, serviceName)
	return newCtx, conn, err
}

func (d *director) Release(ctx context.Context, conn *grpc.ClientConn) {
	serviceName := ctx.Value(serviceNameKey{}).(string)
	d.cr.Release(serviceName, conn)
}

func main() {
	// proxy must know etcd to do service address lookup, and also self address registration
	etcd := registry.NewEtcd([]string{"127.0.0.1:2379"})

	// the backend connector which dials by service name
	cr := connector.NewCachingConnector(
		connector.WithDialer(
			func(ctx context.Context, target string, _ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
				return etcd.Dial(ctx, target, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second), grpc.WithCodec(proxy.Codec()))
			}))
	cr.OnConnect = func(addr string) {
		log.Printf("@_OnConnect addr = %q", addr)
	}
	cr.OnCacheMiss = func(addr string) {
		log.Printf("@_OnCacheMiss addr = %q", addr)
	}
	cr.OnConnectionCountUpdate = func(count int) {
		log.Printf("@_OnConnectionCountUpdate count = %d", count)
	}

	// proxy is also a network listener and grpc server
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	director := &director{cr: cr}
	gserver := grpc.NewServer(
		grpc.CustomCodec(proxy.Codec()),
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)
	//proxy := NewProxy(lis)

	// start the serving in the background
	stopped := make(chan struct{})
	go func() {
		if err := gserver.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		stopped <- struct{}{}
	}()

	// register proxy service
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	ticker := time.Tick(60 * time.Second)
	lease := etcd.Grant(65 * time.Second)
	etcd.Add(lease, serviceName, lis.Addr().String())

heartbeatLoop:
	for {
		select {
		case <-ticker:
			etcd.KeepAlive(lease)
			if expired := cr.Expire(); len(expired) > 0 {
				log.Printf("director expire %v", expired)
			}
		case <-term:
			etcd.Revoke(lease)
			gserver.Stop()
			break heartbeatLoop
		}
	}
	<-stopped
	log.Printf("bye bye")
}
