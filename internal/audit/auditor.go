package audit

import (
	"github.com/kyokan/chaind/pkg"
)

type Auditor interface {
	RecordRequest(req *pkg.WrappedRequest, reqType pkg.BackendType) error
}
