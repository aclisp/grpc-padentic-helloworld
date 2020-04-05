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
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "grpc-padentic-helloworld/helloworld"
	"grpc-padentic-helloworld/registry"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	serviceName = "com.github.aclisp.grpcpadentic.helloworld"
	address     = "127.0.0.1:0"
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
	listener      net.Listener
	stop          chan struct{}
	sayHelloCount int
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, req *pb.HelloRequest) (res *pb.HelloReply, err error) {
	md, _ := metadata.FromIncomingContext(ctx)
	forwarded := md["x-forwarded-for"]
	log.Printf("Received: %v from %v", req.GetName(), forwarded)
	res = &pb.HelloReply{Message: "Hello " + req.GetName() + " " + s.listener.Addr().String()}
	s.sayHelloCount++
	if s.sayHelloCount%2 == 1 {
		//err = fmt.Errorf("say hello count is %v", s.sayHelloCount)
		err = status.Errorf(codes.Code(100), "say hello count is %v", s.sayHelloCount)
	}
	return res, err
}

func (s *server) SubscribeNotice(req *pb.SubscribeRequest, srv pb.Greeter_SubscribeNoticeServer) (err error) {
	md, _ := metadata.FromIncomingContext(srv.Context())
	forwarded := md["x-forwarded-for"]
	log.Printf("subscribed by %q from %v", req.Identity, forwarded)
	defer func() {
		log.Printf("un-subscribed %q on %v", req.Identity, err)
	}()
	for i := 0; ; i++ {
		msg := fmt.Sprintf("%q notice %q: %d", s.listener.Addr(), req.Identity, i)
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

func newServer(l net.Listener) (s *server) {
	return &server{
		listener: l,
		stop:     make(chan struct{}, 1),
	}
}

func (s *server) Stop() {
	s.stop <- struct{}{}
}

func main() {
	grpc.EnableTracing = true
	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stderr, ioutil.Discard, ioutil.Discard, 99))
	go func() { log.Println(http.ListenAndServe("127.0.0.1:6060", nil)) }()

	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	gserver := grpc.NewServer()
	service := newServer(lis)
	pb.RegisterGreeterServer(gserver, service)

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
			service.Stop()
			gserver.GracefulStop()
			break heartbeatLoop
		}
	}
	<-stopped
	log.Printf("bye bye")
}
