package registry

import "context"

// The registrar provides an interface for service discovery
// and an abstraction over varying implementations
// {consul, etcd, zookeeper, ...}.
type Registrar interface {
	Name() string                                        //return registry name
	Init(...RegistryOption) error                        //init registry
	Register(*Service, ...RegisterOption) error          //register service
	Deregister(*Service, ...DeregisterOption) error      //deregister service
	GetService(string, ...GetOption) ([]*Service, error) //get service list
	Watch(...WatchOption) (Watcher, error)               //watch service
}

// HealthStatus represents the connectivity state of a registry backend.
type HealthStatus int

const (
	// HealthUnknown indicates the registry cannot determine its own state.
	HealthUnknown HealthStatus = iota
	// HealthOK indicates the registry backend is reachable.
	HealthOK
	// HealthUnavailable indicates the registry backend is unreachable.
	HealthUnavailable
)

// HealthChecker is an optional capability interface for registries that can
// report the health of their backend connection. Registries that do not
// implement HealthChecker are treated as healthy by default.
type HealthChecker interface {
	// Status reports the current health of the registry backend.
	Status(ctx context.Context) (HealthStatus, error)
}
