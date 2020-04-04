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
	"grpc-padentic-helloworld/registry"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "grpc-padentic-helloworld/helloworld"

	"google.golang.org/grpc"
)

const (
	serviceName = "greeter_server"
	address     = "127.0.0.1:0"
)

var lis net.Listener

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
	stop chan struct{}
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.GetName())
	return &pb.HelloReply{Message: "Hello " + in.GetName() + " " + lis.Addr().String()}, nil
}

func (s *server) SubscribeNotice(req *pb.SubscribeRequest, srv pb.Greeter_SubscribeNoticeServer) (err error) {
	log.Printf("subscribed by %q", req.Identity)
	defer func() {
		log.Printf("un-subscribed %q on %v", req.Identity, err)
	}()
	for i := 0; ; i++ {
		msg := fmt.Sprintf("%q notice %q: %d", lis.Addr(), req.Identity, i)
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

func NewServer() (s *server) {
	return &server{
		stop: make(chan struct{}, 1),
	}
}

func (s *server) Stop() {
	s.stop <- struct{}{}
}

func main() {
	var err error
	lis, err = net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	gserver := grpc.NewServer()
	service := NewServer()
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
