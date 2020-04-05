package main

import (
	"context"
	rt "grpc-padentic-helloworld/router"
	"grpc-padentic-helloworld/service"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	serviceName = "com.github.aclisp.grpcpadentic.router"
	address     = "127.0.0.1:0"
)

// ClientIdentity is the client ID type
type ClientIdentity string

type router struct {
	rt.UnimplementedRouterServer

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

func newRouter() (r *router) {
	return &router{
		routes: make(map[ClientIdentity]*rt.Route),
	}
}

func main() {
	go func() { log.Println(http.ListenAndServe("127.0.0.1:6063", nil)) }()

	service := service.New(serviceName, address, 10*time.Second)
	router := newRouter()
	rt.RegisterRouterServer(service.Server(), router)
	service.Run()
}
