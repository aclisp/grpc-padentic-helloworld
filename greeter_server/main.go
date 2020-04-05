/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

//go:generate protoc -I ../helloworld --go_out=plugins=grpc:../helloworld ../helloworld/helloworld.proto

// Package main implements a server for Greeter service.
package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "grpc-padentic-helloworld/helloworld"
	rt "grpc-padentic-helloworld/router"
	"grpc-padentic-helloworld/service"
	"log"
	"net/http"
	"time"
)

const (
	serviceName = "com.github.aclisp.grpcpadentic.helloworld"
	address     = "127.0.0.1:0"
)

// server is used to implement helloworld.GreeterServer.
type greeter struct {
	pb.UnimplementedGreeterServer
	service       service.Service
	stop          chan struct{}
	sayHelloCount int
	router        rt.RouterClient
}

func (s *greeter) getRoute(ctx context.Context) {
	if route, err := s.router.GetRoute(ctx, &rt.GetRouteReq{}); err != nil {
		log.Printf(" --- route --- error: %v", err)
	} else if len(route.Routes) == 0 {
		log.Printf(" --- route --- no routes")
	} else {
		for _, x := range route.Routes {
			log.Printf(" --- route --- %q -> %q", x.ClientIdentity, x.ServerAddress)
		}
	}
}

func (s *greeter) addRoute(ctx context.Context, id, addr string) {
	if _, err := s.router.AddRoute(ctx, &rt.AddRouteReq{
		Route: &rt.Route{
			ClientIdentity: id,
			ServerAddress:  addr,
		},
	}); err != nil {
		log.Printf(" --- add route --- error: %v", err)
	}
}

func (s *greeter) delRoute(ctx context.Context, id string) {
	if _, err := s.router.DelRoute(ctx, &rt.DelRouteReq{
		Route: &rt.Route{
			ClientIdentity: id,
		},
	}); err != nil {
		log.Printf(" --- del route --- error: %v", err)
	}
}

// SayHello implements helloworld.GreeterServer
func (s *greeter) SayHello(ctx context.Context, req *pb.HelloRequest) (res *pb.HelloReply, err error) {

	// first we do a server-to-server RPC to know the trace
	s.getRoute(ctx)

	md, _ := metadata.FromIncomingContext(ctx)
	forwarded := md["x-forwarded-for"]
	log.Printf("Received: %v from %v", req.GetName(), forwarded)
	res = &pb.HelloReply{Message: "Hello " + req.GetName() + " " + s.service.Address()}
	s.sayHelloCount++
	if s.sayHelloCount%2 == 1 {
		//err = fmt.Errorf("say hello count is %v", s.sayHelloCount)
		err = status.Errorf(codes.Code(100), "say hello count is %v", s.sayHelloCount)
	}
	return res, err
}

func (s *greeter) SubscribeNotice(req *pb.SubscribeRequest, srv pb.Greeter_SubscribeNoticeServer) (err error) {
	md, _ := metadata.FromIncomingContext(srv.Context())
	forwarded := md["x-forwarded-for"]
	log.Printf("subscribed by %q from %v", req.Identity, forwarded)
	s.addRoute(context.TODO(), req.Identity, s.service.Address())
	defer func() {
		log.Printf("un-subscribed %q on %v", req.Identity, err)
		s.delRoute(context.TODO(), req.Identity)
	}()
	for i := 0; ; i++ {
		msg := fmt.Sprintf("%q notice %q: %d", s.service.Address(), req.Identity, i)
		if err := srv.Send(&pb.Notice{Message: msg}); err != nil {
			return err
		}
		select {
		case <-s.stop:
			return fmt.Errorf("server stopped")
		case <-time.After(2 * time.Second):
		}
	}
}

func newGreeter(s service.Service) (g *greeter) {
	g = &greeter{
		service: s,
		stop:    make(chan struct{}),
	}

	conn, err := s.Registry().Dial(context.Background(), "com.github.aclisp.grpcpadentic.router", grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	g.router = rt.NewRouterClient(conn)
	return g
}

func (s *greeter) Stop() {
	close(s.stop)
}

func main() {
	// for pprof and trace
	go func() { log.Println(http.ListenAndServe("127.0.0.1:6060", nil)) }()

	service := service.New(serviceName, address, 10*time.Second)
	greeter := newGreeter(service)
	service.SetInterrupter(func() {
		greeter.Stop()
	})
	pb.RegisterGreeterServer(service.Server(), greeter)
	service.Run()
}
