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
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.etcd.io/etcd/clientv3"
	etcdnaming "go.etcd.io/etcd/clientv3/naming"
	"google.golang.org/grpc"
	"google.golang.org/grpc/naming"
	pb "grpc-padentic-helloworld/helloworld"
)

const (
	service = "greeter_server"
	address = "127.0.0.1:0"
)

var lis net.Listener

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.GetName())
	return &pb.HelloReply{Message: "Hello " + in.GetName() + " " + lis.Addr().String()}, nil
}

func main() {
	var err error
	lis, err = net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})

	stopped := make(chan struct{})
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		stopped <- struct{}{}
	}()

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	ticker := time.Tick(10 * time.Second)
	etcd := etcdConnect([]string{"127.0.0.1:2379"})
	lease := etcdGrant(etcd, 15)
	etcdLeaseAdd(etcd, lease, service, lis.Addr().String())

heartbeatLoop:
	for {
		select {
		case <-ticker:
			etcdKeepAlive(etcd, lease)
		case <-term:
			etcdRevoke(etcd, lease)
			s.GracefulStop()
			break heartbeatLoop
		}
	}
	<-stopped
	log.Printf("bye bye")
}

func etcdConnect(endpoints []string) *clientv3.Client {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("etcd: failed to connect: %v", err)
	}
	return cli
}

func etcdGrant(c *clientv3.Client, ttlSeconds int64) clientv3.LeaseID {
	var lid clientv3.LeaseID
	if lgr, err := c.Grant(context.Background(), ttlSeconds); err != nil {
		log.Fatalf("etcd: failed to grant lease: %v", err)
	} else {
		lid = lgr.ID
	}
	return lid
}

func etcdLeaseAdd(c *clientv3.Client, lid clientv3.LeaseID, service, addr string) {
	r := &etcdnaming.GRPCResolver{Client: c}
	if err := r.Update(c.Ctx(), service, naming.Update{Op: naming.Add, Addr: addr}, clientv3.WithLease(lid)); err != nil {
		log.Fatalf("etcd: failed to add service %q addr %q with lease %v: %v", service, addr, lid, err)
	}
	log.Printf("etcd: add service %q addr %q with lease %v", service, addr, lid)
}

func etcdRevoke(c *clientv3.Client, lid clientv3.LeaseID) {
	c.Revoke(context.Background(), lid)
	log.Printf("etcd: revoke lease %v", lid)
}

func etcdKeepAlive(c *clientv3.Client, lid clientv3.LeaseID) {
	if res, err := c.KeepAliveOnce(context.Background(), lid); err != nil {
		// fail fast
		log.Fatalf("etcd: failed to keep lease %v alive: %v", lid, err)
	} else {
		log.Printf("etcd: keep lease %v alive: ttl %v", lid, res.TTL)
	}
}
