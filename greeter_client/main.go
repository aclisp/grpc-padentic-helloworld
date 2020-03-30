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

// Package main implements a client for Greeter service.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"go.etcd.io/etcd/clientv3"
	etcdnaming "go.etcd.io/etcd/clientv3/naming"
	"google.golang.org/grpc"
	pb "grpc-padentic-helloworld/helloworld"
)

const (
	service     = "greeter_server"
	defaultName = "world"
)

func main() {
	// Set up a connection to the server.
	etcd := etcdConnect([]string{"127.0.0.1:2379"})
	conn, err := etcdDial(etcd, service)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewGreeterClient(conn)

	// Contact the server and print out its response.
	name := defaultName
	if len(os.Args) > 1 {
		name = os.Args[1]
	}

	sayHello := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		r, err := c.SayHello(ctx, &pb.HelloRequest{Name: name})
		if err != nil {
			log.Printf("!xxxxxx! could not greet: %v", err)
		} else {
			log.Printf("!oooooo! Greeting: %s", r.GetMessage())
		}
	}

	for {
		sayHello()
		time.Sleep(5 * time.Second)
	}
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

func etcdDial(c *clientv3.Client, service string) (*grpc.ClientConn, error) {
	r := &etcdnaming.GRPCResolver{Client: c}
	b := grpc.RoundRobin(r)
	return grpc.Dial(service, grpc.WithBalancer(b), grpc.WithInsecure())
}
