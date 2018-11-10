package audit

import (
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/config"
	"github.com/kyokan/chaind/pkg"
	"encoding/json"
	"github.com/pkg/errors"
)

type LogAuditor struct {
	logger log15.Logger
}

func NewLogAuditor(cfg *config.LogAuditorConfig) (Auditor, error) {
	if cfg == nil {
		return nil, errors.New("no log auditor config defined")
	}

	logger := log15.New()
	hdlr, err := log15.FileHandler(cfg.LogFile, log15.LogfmtFormat())
	if err != nil {
		return nil, err
	}
	logger.SetHandler(hdlr)

	return &LogAuditor{
		logger: logger,
	}, nil
}

func (l *LogAuditor) RecordRequest(req *pkg.WrappedRequest, reqType pkg.BackendType) error {
	if reqType == pkg.EthereumBackendType {
		return l.recordETHRequest(req)
	}

	return nil
}

func (l *LogAuditor) recordETHRequest(req *pkg.WrappedRequest) error {
	var body pkg.EthereumRPCRequest
	err := json.Unmarshal(req.Body(), &body)
	if err != nil {
		l.logger.Error(
			"received request with invalid JSON body",
			MergeLogKeys(req, "type", pkg.EthereumBackendType)...,
		)
		return nil
	}

	params, err := json.Marshal(body.Params)
	if err != nil {
		return err
	}
	l.logger.Info(
		"received JSON-RPC request",
		MergeLogKeys(req, "rpc_method", body.Method, "rpc_params", string(params))...,
	)
	return nil
}

func MergeLogKeys(req *pkg.WrappedRequest, keys... interface{}) []interface{} {
	defaults := []interface{}{
		"remote_addr",
		req.RemoteAddr(),
		"user_agent",
		req.Header().Get("user-agent"),
	}

	return append(keys, defaults...)
}
