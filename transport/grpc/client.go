package grpc

import (
	"context"

	"github.com/sjlit/aeus/transport/grpc/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
)

// dial new gRPC "channel" for the target URI provided.
func Dial(ctx context.Context, target string, cbs ...ClientOption) (conn *grpc.ClientConn, err error) {
	opts := &clientOptions{}
	for _, cb := range cbs {
		cb(opts)
	}
	dialOpts := make([]grpc.DialOption, 0, 10)
	if opts.registry != nil {
		dialOpts = append(
			dialOpts,
			grpc.WithResolvers(
				resolver.New(
					resolver.WithRegistry(opts.registry),
				),
			),
		)
	}
	if opts.tls != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(opts.tls)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(grpcinsecure.NewCredentials()))
	}
	if opts.dialOptions != nil {
		dialOpts = append(dialOpts, opts.dialOptions...)
	}
	conn, err = grpc.NewClient(target, dialOpts...)
	return
}
