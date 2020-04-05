package service

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"grpc-padentic-helloworld/registry"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Service is an interface stands for grpc micro services
type Service interface {
	// Name is the service name, which is the same as protocol package name
	Name() string
	// Address is the service listening address
	Address() string
	// Registry is the service registry
	Registry() *registry.Etcd
	// Server is the gRPC server
	Server() *grpc.Server
	// SetInterrupter sets a func which is called just before graceful stop
	SetInterrupter(func())
	// SetHeartbeatTask sets a func repeatedly called on heartbeat
	SetHeartbeatTask(func())
	// SetServer replace the default gRPC server, used to specify custom server options
	SetServer(*grpc.Server)
	// Run the service until be killed, or quit exceptionally
	Run()
}

type service struct {
	name            string        // assigned by New
	address         string        // assigned by New
	heartbeat       time.Duration // assigned by New
	resolvedAddress string        // resolved address after net.Listen
	listener        net.Listener
	etcd            *registry.Etcd
	server          *grpc.Server
	interrupter     func()
	heartbeater     func()
}

// New a service, specifying
//   - name, which is the same as protocol package name
//   - address, which is used for tcp listen
//   - heartbeat interval, which is to repeatedly run critical tasks
func New(name, address string, heartbeat time.Duration) Service {
	// setup grpc tracing and logging
	grpc.EnableTracing = true
	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stderr, ioutil.Discard, ioutil.Discard, 99))

	s := &service{
		name:      name,
		address:   address,
		heartbeat: heartbeat,
	}

	log.Printf("new service %q on address %q with heartbeat of %v", s.name, s.address, s.heartbeat)
	var err error
	if s.listener, err = net.Listen("tcp", s.address); err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s.resolvedAddress = s.listener.Addr().String()
	s.etcd = registry.NewEtcd([]string{"127.0.0.1:2379"})
	s.server = grpc.NewServer()
	return s
}

func (s *service) Name() string {
	return s.name
}

func (s *service) Address() string {
	if s.resolvedAddress != "" {
		return s.resolvedAddress
	}
	return s.address
}

func (s *service) Registry() *registry.Etcd {
	return s.etcd
}

func (s *service) Server() *grpc.Server {
	return s.server
}

func (s *service) SetInterrupter(interrupter func()) {
	s.interrupter = interrupter
}

func (s *service) SetHeartbeatTask(heartbeater func()) {
	s.heartbeater = heartbeater
}

func (s *service) SetServer(server *grpc.Server) {
	s.server = server
}

func (s *service) Run() {
	// start serving in the background
	stopped := make(chan struct{})
	go func() {
		if err := s.server.Serve(s.listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		stopped <- struct{}{}
	}()

	// register myself
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	ticker := time.Tick(s.heartbeat)
	lease := s.etcd.Grant(s.heartbeat + 5*time.Second)
	s.etcd.Add(lease, s.name, s.resolvedAddress)

	// start the heartbeat loop
	log.Printf("service %q running on address %q with heartbeat of %v", s.name, s.resolvedAddress, s.heartbeat)

heartbeatLoop:
	for {
		select {
		case <-ticker:
			s.etcd.KeepAlive(lease)
			if s.heartbeater != nil {
				s.heartbeater()
			}
		case <-term:
			s.etcd.Revoke(lease)
			if s.interrupter != nil {
				s.interrupter()
			}
			s.server.GracefulStop()
			break heartbeatLoop
		}
	}
	<-stopped
	log.Printf("bye bye")
}
