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

type Etcd struct {
	C *clientv3.Client
}

func NewEtcd(endpoints []string) (e *Etcd) {
	e = &Etcd{}
	e.Connect(endpoints)
	return e
}

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

func (e *Etcd) Grant(ttl time.Duration) (lid clientv3.LeaseID) {
	if lgr, err := e.C.Grant(context.Background(), int64(ttl.Seconds())); err != nil {
		log.Fatalf("etcd: failed to grant lease: %v", err)
	} else {
		lid = lgr.ID
	}
	return lid
}

func (e *Etcd) Add(lid clientv3.LeaseID, service, addr string) {
	r := &etcdnaming.GRPCResolver{Client: e.C}
	if err := r.Update(e.C.Ctx(), service, naming.Update{Op: naming.Add, Addr: addr}, clientv3.WithLease(lid)); err != nil {
		log.Fatalf("etcd: failed to add service %q addr %q with lease %v: %v", service, addr, lid, err)
	}
	log.Printf("etcd: add service %q addr %q with lease %v", service, addr, lid)
}

func (e *Etcd) Revoke(lid clientv3.LeaseID) {
	e.C.Revoke(context.Background(), lid)
	log.Printf("etcd: revoke lease %v", lid)
}

func (e *Etcd) KeepAlive(lid clientv3.LeaseID) {
	if _, err := e.C.KeepAliveOnce(context.Background(), lid); err != nil {
		// fail fast
		log.Fatalf("etcd: failed to keep lease %v alive: %v", lid, err)
	} else {
		//log.Printf("etcd: keep lease %v alive: ttl %v", lid, res.TTL)
	}
}

func (e *Etcd) Dial(service string) (*grpc.ClientConn, error) {
	r := &etcdnaming.GRPCResolver{Client: e.C}
	b := grpc.RoundRobin(r)
	conn, err := grpc.Dial(service, grpc.WithBalancer(b), grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second))
	if err != nil {
		return nil, fmt.Errorf("etcd: failed to dial service %q: %v", service, err)
	}
	return conn, nil
}