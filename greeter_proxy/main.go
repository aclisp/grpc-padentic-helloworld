package main

import (
	"context"
	"github.com/vgough/grpc-proxy/connector"
	"github.com/vgough/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"grpc-padentic-helloworld/service"
	"log"
	"net/http"
	"strings"
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

func (d *director) Connect(ctx context.Context, method string) (context.Context, *grpc.ClientConn, error) {
	log.Printf("director connect %q", method)
	// turns "/pkg.Service/GetFoo" into "pkg"
	method = strings.TrimPrefix(method, "/") // remove leading slash
	if i := strings.Index(method, "/"); i >= 0 {
		method = method[:i] // remove everything from second slash
	}
	if i := strings.LastIndex(method, "."); i >= 0 {
		method = method[:i] // cut down from last dotted component
	}
	// use "pkg" as service name
	conn, err := d.cr.Dial(ctx, method)
	newCtx := context.WithValue(ctx, serviceNameKey{}, method)
	return newCtx, conn, err
}

func (d *director) Release(ctx context.Context, conn *grpc.ClientConn) {
	serviceName := ctx.Value(serviceNameKey{}).(string)
	d.cr.Release(serviceName, conn)
}

func main() {
	// setup grpc tracing and logging
	go func() { log.Println(http.ListenAndServe("127.0.0.1:6061", nil)) }()

	// proxy must know etcd to do service address lookup, and also self address registration
	service := service.New(serviceName, address, 60*time.Second)

	// the backend connector which dials by service name
	cr := connector.NewCachingConnector(
		connector.WithDialer(
			func(ctx context.Context, target string, _ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
				return service.Registry().Dial(ctx, target, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second), grpc.WithCodec(proxy.Codec()))
			}))
	cr.OnConnect = func(addr string) {
		log.Printf("connector event OnConnect addr = %q", addr)
	}
	cr.OnCacheMiss = func(addr string) {
		log.Printf("connector event OnCacheMiss addr = %q", addr)
	}
	cr.OnConnectionCountUpdate = func(count int) {
		log.Printf("connector event OnConnectionCountUpdate count = %d", count)
	}

	// proxy is also a network listener and grpc server
	director := &director{cr: cr}
	gserver := grpc.NewServer(
		grpc.CustomCodec(proxy.Codec()),
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)
	service.SetServer(gserver)
	service.SetHeartbeatTask(func() {
		if expired := cr.Expire(); len(expired) > 0 {
			log.Printf("director expire %v", expired)
		}
	})
	service.SetInterrupter(func() {
		gserver.Stop()
	})

	// start the serving in the background
	service.Run()
}
