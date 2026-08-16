package pb

import (
	"google.golang.org/protobuf/types/known/emptypb"
)

// Empty aliases google.protobuf.Empty so the protoc-gen-go-aeus wrapper
// code generated for user.proto can refer to a local type. protoc-gen-go
// does not currently emit a package-level alias for empty.proto, so this
// hand-written bridge keeps generated files untouched.
type Empty = emptypb.Empty
