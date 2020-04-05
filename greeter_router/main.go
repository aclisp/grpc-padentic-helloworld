package main

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"grpc-padentic-helloworld/registry"
	rt "grpc-padentic-helloworld/router"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	serviceName = "com.github.aclisp.grpcpadentic.router"
	address     = "127.0.0.1:0"
)

type ClientIdentity string

type router struct {
	rt.UnimplementedRouterServer
	listener net.Listener
	stop     chan struct{}

	mux    sync.Mutex // protects routes
	routes map[ClientIdentity]*rt.Route
}

func (r *router) GetRoute(ctx context.Context, req *rt.GetRouteReq) (*rt.GetRouteRes, error) {
	res := new(rt.GetRouteRes)

	r.mux.Lock()
	res.Routes = make([]*rt.Route, 0, len(r.routes))
	for _, r := range r.routes {
		res.Routes = append(res.Routes, r)
	}
	r.mux.Unlock()

	sort.Slice(res.Routes, func(i, j int) bool {
		return res.Routes[i].ClientIdentity < res.Routes[j].ClientIdentity
	})
	return res, nil
}

func (r *router) AddRoute(ctx context.Context, req *rt.AddRouteReq) (*rt.AddRouteRes, error) {
	r.mux.Lock()
	defer r.mux.Unlock()

	r.routes[ClientIdentity(req.Route.ClientIdentity)] = req.Route
	return new(rt.AddRouteRes), nil
}

func (r *router) DelRoute(ctx context.Context, req *rt.DelRouteReq) (*rt.DelRouteRes, error) {
	r.mux.Lock()
	defer r.mux.Unlock()

	delete(r.routes, ClientIdentity(req.Route.ClientIdentity))
	return new(rt.DelRouteRes), nil
}

func newRouter(l net.Listener) (r *router) {
	return &router{
		listener: l,
		stop:     make(chan struct{}, 1),
		routes:   make(map[ClientIdentity]*rt.Route),
	}
}

func (r *router) Stop() {
	r.stop <- struct{}{}
}

func main() {
	grpc.EnableTracing = true
	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stderr, ioutil.Discard, ioutil.Discard, 99))
	go func() { log.Println(http.ListenAndServe("127.0.0.1:6063", nil)) }()

	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	gserver := grpc.NewServer()
	router := newRouter(lis)
	rt.RegisterRouterServer(gserver, router)

	stopped := make(chan struct{})
	go func() {
		if err := gserver.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		stopped <- struct{}{}
	}()

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	ticker := time.Tick(10 * time.Second)
	etcd := registry.NewEtcd([]string{"127.0.0.1:2379"})
	lease := etcd.Grant(15 * time.Second)
	etcd.Add(lease, serviceName, lis.Addr().String())

heartbeatLoop:
	for {
		select {
		case <-ticker:
			etcd.KeepAlive(lease)
		case <-term:
			etcd.Revoke(lease)
			router.Stop()
			gserver.GracefulStop()
			break heartbeatLoop
		}
	}
	<-stopped
	log.Printf("bye bye")
}
