package proxy

import (
	"net/http"
	"encoding/json"
	"github.com/kyokan/chaind/pkg"
	"github.com/kyokan/chaind/internal/cache"
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/log"
	"context"
	"time"
	"net/url"
	"io/ioutil"
	"bytes"
	"net/http/httputil"
	"github.com/kyokan/chaind/pkg/rpc"
	"fmt"
	"strconv"
	"github.com/pkg/errors"
	"reflect"
	"github.com/kyokan/chaind/internal/audit"
)

const FinalityDepth = 12

type beforeFunc func(res http.ResponseWriter, req *http.Request, rpcReq *rpc.JSONRPCReq) bool
type afterFunc func(res *pkg.InterceptedResponse, req *http.Request) error

type handler struct {
	before beforeFunc
	after  afterFunc
}

type EthHandler struct {
	cacher   cache.Cacher
	auditor  audit.Auditor
	handlers map[string]*handler
	logger   log15.Logger
}

func NewEthHandler(cacher cache.Cacher, auditor audit.Auditor) *EthHandler {
	h := &EthHandler{
		cacher:  cacher,
		auditor: auditor,
		logger:  log.NewLog("proxy/eth_handler"),
	}
	h.handlers = map[string]*handler{
		"eth_getBlockByNumber": {
			before: h.hdlGetBlockByNumberBefore,
			after:  h.hdlGetBlockByNumberAfter,
		},
	}
	return h
}

func (h *EthHandler) Handle(res http.ResponseWriter, req *http.Request, backend *pkg.Backend) {
	defer req.Body.Close()
	ctx := req.Context()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		h.logger.Error("failed to read request body", rpc.LogWithRequestID(ctx, "err", err)...)
		return
	}

	var arrReq []interface{}
	var rpcReq rpc.JSONRPCReq
	err = json.Unmarshal(body, &arrReq)
	if err == nil {
		h.logger.Debug("parsing array value", rpc.LogWithRequestID(ctx)...)
		var rpcReqSlice []rpc.JSONRPCReq
		err = json.Unmarshal(body, &rpcReqSlice)
		if err != nil {
			res.WriteHeader(http.StatusBadRequest)
			h.logger.Error("failed to parse request in array", rpc.LogWithRequestID(ctx, "err", err)...)
			return
		}
		rpcReq = rpcReqSlice[0]
	} else {
		err = json.Unmarshal(body, &rpcReq)
		if err != nil {
			res.WriteHeader(http.StatusBadRequest)
			h.logger.Error("failed to parse request", rpc.LogWithRequestID(ctx, "err", err)...)
			return
		}
	}

	err = h.auditor.RecordRequest(req, body, pkg.EthBackend)
	if err != nil {
		h.logger.Error("failed to record audit log for request", rpc.LogWithRequestID(ctx, "err", err)...)
	}

	hdlr := h.handlers[rpcReq.Method]
	handledInBefore := false
	if hdlr != nil && hdlr.before != nil {
		handledInBefore = hdlr.before(res, req, &rpcReq)
	}
	if handledInBefore {
		return
	}

	iceptRes := pkg.InterceptResponse(res)
	u, err := url.Parse(backend.URL)
	if err != nil {
		h.logger.Error("failed to parse backend URL", rpc.LogWithRequestID(ctx, "url", backend.URL, "err", err)...)
		return
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.URL.Path = u.Path
	req.Host = u.Host
	reader := ioutil.NopCloser(bytes.NewBuffer(body))
	req.Body = reader
	proxy := httputil.NewSingleHostReverseProxy(u)
	timeoutCtx, _ := context.WithTimeout(ctx, time.Second)
	req = req.WithContext(timeoutCtx)
	proxy.ServeHTTP(iceptRes, req)
	iceptRes.Flush()
	if err != nil {
		h.logger.Error("failed to flush intercepted request", rpc.LogWithRequestID(ctx, "err", err)...)
		failWithInternalError(res, rpcReq.Id, err)
		return
	}

	if hdlr != nil && hdlr.after != nil && iceptRes.IsOK() {
		if err := hdlr.after(iceptRes, req); err != nil {
			h.logger.Error("request post-processing failed", rpc.LogWithRequestID(ctx, "err", err)...)
		}
	} else {
		h.logger.Debug("no post-processor found", rpc.LogWithRequestID(ctx)...)
	}
}

func (h *EthHandler) hdlGetBlockByNumberBefore(res http.ResponseWriter, req *http.Request, rpcReq *rpc.JSONRPCReq) bool {
	ctx := req.Context()
	h.logger.Debug("pre-processing eth_getBlockByNumber", rpc.LogWithRequestID(ctx)...)
	params := rpcReq.Params
	paramCount := len(params)
	if paramCount == 0 {
		return false
	}

	blockNum, ok := params[0].(string)
	if !ok {
		h.logger.Debug("encountered invalid block number param, bailing", rpc.LogWithRequestID(ctx, "block_num", params[0])...)
		return false
	}

	var includeBodies bool
	if paramCount == 2 {
		testIncludeBodies, ok := params[1].(bool)
		if !ok {
			h.logger.Debug("encountered invalid include bodies param, bailing", rpc.LogWithRequestID(ctx, "include_bodies", params[1])...)
			return false
		}
		includeBodies = testIncludeBodies
	}

	cacheKey := blockNumCacheKey(blockNum, includeBodies)
	h.logger.Debug("checking block number cache", rpc.LogWithRequestID(ctx, "cache_key", cacheKey)...)
	cached, err := h.cacher.Get(cacheKey)
	if err == nil && cached != nil {
		err = writeResponse(res, rpcReq.Id, cached)
		if err != nil {
			h.logger.Error("failed to write cached response", "err", err)
			return false
		}
		h.logger.Debug("found cached block number response, sending", rpc.LogWithRequestID(ctx)...)
		return true
	}

	h.logger.Debug("found no blocks in block number cache", rpc.LogWithRequestID(ctx)...)
	return false
}

func (h *EthHandler) hdlGetBlockByNumberAfter(res *pkg.InterceptedResponse, req *http.Request) error {
	ctx := req.Context()
	h.logger.Debug("post-processing eth_getBlockByNumber", rpc.LogWithRequestID(ctx)...)
	body := res.Body()
	var parsed rpc.JSONRPCRes
	err := json.Unmarshal(body, &parsed)
	if err != nil {
		h.logger.Debug("post-processing failed while unmarshalling response", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	result, ok := parsed.Result.(map[string]interface{})
	if !ok {
		return errors.New("failed to parse RPC results")
	}
	blockNum, ok := result["number"].(string)
	if !ok {
		return errors.New("failed to parse block number from RPC results")
	}
	var includeBodies bool
	transactions, ok := result["transactions"].([]interface{})
	if !ok {
		return errors.New("failed to parse transactions from RPC results")
	}
	if len(transactions) == 0 {
		includeBodies = false
	} else {
		includeBodies = reflect.TypeOf(transactions[0]).Kind() != reflect.String
	}

	serialized, err := json.Marshal(result)
	if err != nil {
		h.logger.Debug("post-processing failed while marshalling to cache", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	cacheKey := blockNumCacheKey(blockNum, includeBodies)
	err = h.cacher.SetEx(cacheKey, serialized, time.Hour)
	if err != nil {
		h.logger.Debug("post-processing failed while writing to cache", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	h.logger.Debug("stored request in block number cache", rpc.LogWithRequestID(ctx, "cache_key", cacheKey, "size", len(serialized))...)
	return nil
}

func writeResponse(res http.ResponseWriter, id interface{}, data []byte) error {
	var result map[string]interface{}
	err := json.Unmarshal(data, &result)
	if err != nil {
		return err
	}
	outJson := &rpc.JSONRPCRes{
		Jsonrpc: rpc.JSONRPC2,
		Id:      id,
		Result:  result,
	}

	out, err := json.Marshal(outJson)
	if err != nil {
		return err
	}
	res.Write(out)
	return nil
}

func failWithInternalError(res http.ResponseWriter, id interface{}, err error) {
	failRequest(res, id, -32600, err.Error())
}

func failRequest(res http.ResponseWriter, id interface{}, code int, msg string) {
	outJson := &rpc.JSONRPCErrorRes{
		Jsonrpc: rpc.JSONRPC2,
		Id:      id,
		Error: &rpc.JSONRPCErrorData{
			Code:    code,
			Message: msg,
		},
	}
	out, err := json.Marshal(outJson)
	if err != nil {
		out = []byte(rpc.InternalError)
	}

	res.WriteHeader(http.StatusOK)
	res.Write(out)
}

func blockNumCacheKey(blockNum string, includeBodies bool) string {
	return fmt.Sprintf("block:%s:%s", blockNum, strconv.FormatBool(includeBodies))
}
