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
	"fmt"
	"grpc-padentic-helloworld/registry"
	"log"
	"os"
	"time"

	pb "grpc-padentic-helloworld/helloworld"
)

const (
	service     = "greeter_server"
	defaultName = "world"
)

func main() {
	// Set up a connection to the server.
	etcd := registry.NewEtcd([]string{"127.0.0.1:2379"})
	conn, err := etcd.Dial(service)
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

	// sayHello loop in the background
	go func() {
		for {
			sayHello(c, name)
			time.Sleep(50 * time.Second)
		}
	}()

	// subscribe server notice
	for {
		stream, err := c.SubscribeNotice(context.Background(), &pb.SubscribeRequest{
			Identity: fmt.Sprintf("client-%d", os.Getpid()),
		})
		if err != nil {
			log.Printf("can not subscribe: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for {
			notice, err := stream.Recv()
			if err != nil {
				log.Printf("subscribe stream error: %v, retrying", err)
				break
			}
			log.Printf("got notice: %v", notice)
		}
	}
}

func sayHello(greeter pb.GreeterClient, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := greeter.SayHello(ctx, &pb.HelloRequest{Name: name})
	if err != nil {
		log.Printf("!xxxxxx! could not greet: %v", err)
	} else {
		log.Printf("!oooooo! Greeting: %s", r.GetMessage())
	}
}
