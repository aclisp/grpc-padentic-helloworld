package registry

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"time"

	"go.etcd.io/etcd/clientv3"
	etcdnaming "go.etcd.io/etcd/clientv3/naming"
	"google.golang.org/grpc/naming"
)

// Etcd is a service registry using etcd as the storage
type Etcd struct {
	C *clientv3.Client
}

// NewEtcd creates an Etcd instance by using Connect
func NewEtcd(endpoints []string) (e *Etcd) {
	e = &Etcd{}
	e.Connect(endpoints)
	return e
}

// Connect to etcd with endpoints and a default timeout, block until connected
func (e *Etcd) Connect(endpoints []string) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
		DialOptions: []grpc.DialOption{grpc.WithBlock()},
	})
	if err != nil {
		log.Fatalf("etcd: failed to connect to endpoints %v: %v", endpoints, err)
	}
	e.C = cli
}

// Grant a lease with ttl
func (e *Etcd) Grant(ttl time.Duration) (lid clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if lgr, err := e.C.Grant(ctx, int64(ttl.Seconds())); err != nil {
		log.Fatalf("etcd: failed to grant lease: %v", err)
	} else {
		lid = lgr.ID
	}
	return lid
}

// Add register service address with lease
func (e *Etcd) Add(lid clientv3.LeaseID, service, addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := &etcdnaming.GRPCResolver{Client: e.C}
	if err := r.Update(ctx, service, naming.Update{Op: naming.Add, Addr: addr}, clientv3.WithLease(lid)); err != nil {
		log.Fatalf("etcd: failed to add service %q addr %q with lease %v: %v", service, addr, lid, err)
	}
	log.Printf("etcd: add service %q addr %q with lease %v", service, addr, lid)
}

// Revoke the lease, so that all of its attached services are deleted
func (e *Etcd) Revoke(lid clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	e.C.Revoke(ctx, lid)
	log.Printf("etcd: revoke lease %v", lid)
}

// KeepAlive keeps the lease alive, refreshing its TTL
func (e *Etcd) KeepAlive(lid clientv3.LeaseID) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := e.C.KeepAliveOnce(ctx, lid); err != nil {
		// fail fast
		log.Fatalf("etcd: failed to keep lease %v alive: %v", lid, err)
	} else {
		//log.Printf("etcd: keep lease %v alive: ttl %v", lid, res.TTL)
	}
}

// Dial a grpc client connection by service name
func (e *Etcd) Dial(ctx context.Context, service string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	r := &etcdnaming.GRPCResolver{Client: e.C}
	b := grpc.RoundRobin(r)
	conn, err := grpc.DialContext(ctx, service, append(opts, grpc.WithBalancer(b))...)
	if err != nil {
		return nil, fmt.Errorf("etcd: failed to dial service %q: %v", service, err)
	}
	return conn, nil
}
