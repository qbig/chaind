package proxy

import (
	"net/http"
	"encoding/json"
	"github.com/kyokan/chaind/pkg"
	"github.com/kyokan/chaind/internal/cache"
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/log"
	"time"
	"io/ioutil"
	"bytes"
	"github.com/kyokan/chaind/pkg/rpc"
	"fmt"
	"strconv"
	"github.com/pkg/errors"
	"reflect"
	"github.com/kyokan/chaind/internal/audit"
)

type beforeFunc func(res http.ResponseWriter, req *http.Request, rpcReq *rpc.JSONRPCReq) bool
type afterFunc func(body []byte, req *http.Request) error

type handler struct {
	before beforeFunc
	after  afterFunc
}

type EthHandler struct {
	cacher   cache.Cacher
	auditor  audit.Auditor
	fHelper  *FinalizationHelper
	handlers map[string]*handler
	logger   log15.Logger
	client   *http.Client
}

func NewEthHandler(cacher cache.Cacher, auditor audit.Auditor, fHelper *FinalizationHelper) *EthHandler {
	h := &EthHandler{
		cacher:  cacher,
		auditor: auditor,
		fHelper: fHelper,
		logger:  log.NewLog("proxy/eth_handler"),
		client: &http.Client{
			Timeout: time.Second,
		},
	}
	h.handlers = map[string]*handler{
		"eth_getBlockByNumber": {
			before: h.hdlGetBlockByNumberBefore,
			after:  h.hdlGetBlockByNumberAfter,
		},
		"eth_getTransactionReceipt": {
			before: h.hdlGetTransactionReceiptBefore,
			after:  h.hdlGetTransactionReceiptAfter,
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

	firstChar := string(body[0])
	// check if this is a batch request
	if firstChar == "[" {
		h.logger.Debug("got batch request", rpc.LogWithRequestID(ctx)...)
		var rpcReqs []rpc.JSONRPCReq
		err = json.Unmarshal(body, &rpcReqs)
		if err != nil {
			h.logger.Warn("received mal-formed batch request", rpc.LogWithRequestID(ctx, "err", err)...)
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		batch := pkg.NewBatchResponse(res)
		for _, rpcReq := range rpcReqs {
			h.hdlRPCRequest(batch.ResponseWriter(), req, backend, &rpcReq)
		}
		if err := batch.Flush(); err != nil {
			h.logger.Error("failed to flush batch", rpc.LogWithRequestID(ctx, "err", err)...)
		}

		h.logger.Debug("processed batch request", rpc.LogWithRequestID(ctx, "count", len(rpcReqs))...)
	} else {
		h.logger.Debug("got single request", rpc.LogWithRequestID(ctx, "err", err)...)
		var rpcReq rpc.JSONRPCReq
		err = json.Unmarshal(body, &rpcReq)
		if err != nil {
			h.logger.Warn("received mal-formed request", rpc.LogWithRequestID(ctx, "err", err)...)
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		h.hdlRPCRequest(res, req, backend, &rpcReq)
	}
}

func (h *EthHandler) hdlRPCRequest(res http.ResponseWriter, req *http.Request, backend *pkg.Backend, rpcReq *rpc.JSONRPCReq) {
	ctx := req.Context()
	body, err := json.Marshal(rpcReq)
	if err != nil {
		h.logger.Error("failed to unmarshal request body", rpc.LogWithRequestID(ctx, "err", err)...)
		return
	}

	err = h.auditor.RecordRequest(req, body, pkg.EthBackend)
	if err != nil {
		h.logger.Error("failed to record audit log for request", rpc.LogWithRequestID(ctx, "err", err)...)
	}

	hdlr := h.handlers[rpcReq.Method]
	handledInBefore := false
	if hdlr != nil && hdlr.before != nil {
		handledInBefore = hdlr.before(res, req, rpcReq)
	}
	if handledInBefore {
		h.logger.Debug("request handled in before filter", rpc.LogWithRequestID(ctx)...)
		return
	}

	proxyRes, err := h.client.Post(backend.URL, "application/json", bytes.NewReader(body))
	if err != nil || proxyRes.StatusCode != 200 {
		failRequest(res, rpcReq.Id, -32602, "bad request")
		return
	}
	defer proxyRes.Body.Close()

	resBody, err := ioutil.ReadAll(proxyRes.Body)
	if err != nil {
		failWithInternalError(res, rpcReq.Id, err)
		h.logger.Error("failed to read body", rpc.LogWithRequestID(ctx, "err", err))
	}

	res.Write(resBody)
	if err != nil {
		h.logger.Error("failed to flush proxied request", rpc.LogWithRequestID(ctx, "err", err)...)
		failWithInternalError(res, rpcReq.Id, err)
		return
	}

	var errRes rpc.JSONRPCErrorRes
	isErr := json.Unmarshal(resBody, &errRes) == nil && errRes.Error != nil
	if hdlr != nil && hdlr.after != nil && !isErr {
		if err := hdlr.after(resBody, req); err != nil {
			h.logger.Error("request post-processing failed", rpc.LogWithRequestID(ctx, "err", err)...)
		}
	} else if isErr {
		h.logger.Debug("skipping post-processors for error response", rpc.LogWithRequestID(ctx)...)
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

	if err != nil {
		h.logger.Error("failed to get block from cache", rpc.LogWithRequestID(ctx, "err", err)...)
	}

	h.logger.Debug("found no blocks in block number cache", rpc.LogWithRequestID(ctx)...)
	return false
}

func (h *EthHandler) hdlGetBlockByNumberAfter(body []byte, req *http.Request) error {
	ctx := req.Context()
	h.logger.Debug("post-processing eth_getBlockByNumber", rpc.LogWithRequestID(ctx)...)
	var parsed rpc.JSONRPCRes
	err := json.Unmarshal(body, &parsed)
	if err != nil {
		h.logger.Debug("post-processing failed while unmarshalling response", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	var result map[string]interface{}
	err = json.Unmarshal(parsed.Result, &result)
	if err != nil {
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

	var expiry time.Duration
	if h.fHelper.IsFinalizedHex(blockNum) {
		expiry = time.Hour
	} else {
		h.logger.Debug("not caching un-finalized block")
		return nil
	}

	cacheKey := blockNumCacheKey(blockNum, includeBodies)
	err = h.cacher.SetEx(cacheKey, parsed.Result, expiry)
	if err != nil {
		h.logger.Debug("post-processing failed while writing to cache", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	h.logger.Debug("stored request in block number cache", rpc.LogWithRequestID(ctx, "cache_key", cacheKey, "size", len(parsed.Result))...)
	return nil
}

func (h *EthHandler) hdlGetTransactionReceiptBefore(res http.ResponseWriter, req *http.Request, rpcReq *rpc.JSONRPCReq) bool {
	ctx := req.Context()
	h.logger.Debug("pre-processing eth_getTransactionReceipt", rpc.LogWithRequestID(ctx)...)
	params := rpcReq.Params
	if len(params) == 0 {
		return false
	}

	hash, ok := params[0].(string)
	if !ok {
		h.logger.Debug("encountered invalid tx hash param, bailing", rpc.LogWithRequestID(ctx, "tx_hash", params[0])...)
	}

	cacheKey := txReceiptCacheKey(hash)
	h.logger.Debug("checking transaction receipt cache", rpc.LogWithRequestID(ctx, "cache_key", cacheKey)...)
	cached, err := h.cacher.Get(cacheKey)
	if err == nil && cached != nil {
		err = writeResponse(res, rpcReq.Id, cached)
		if err != nil {
			h.logger.Error("failed to write cached response", "err", err)
			return false
		}
		h.logger.Debug("found cached tx receipt response, sending", rpc.LogWithRequestID(ctx)...)
		return true
	}

	if err != nil {
		h.logger.Error("failed to get tx receipt from cache", rpc.LogWithRequestID(ctx, "err", err)...)
	}

	h.logger.Debug("found no tx receipts in tx receipt cache", rpc.LogWithRequestID(ctx)...)
	return false
}

func (h *EthHandler) hdlGetTransactionReceiptAfter(body []byte, req *http.Request) error {
	ctx := req.Context()
	h.logger.Debug("post-processing eth_getTransactionReceipt", rpc.LogWithRequestID(ctx)...)
	var parsed rpc.JSONRPCRes
	err := json.Unmarshal(body, &parsed)
	if err != nil {
		h.logger.Debug("post-processing failed while unmarshalling response", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}

	if bytes.Equal(parsed.Result, []byte("null")) {
		return nil
	}

	var result map[string]interface{}
	err = json.Unmarshal(parsed.Result, &result)
	if err != nil {
		return errors.New("failed to parse RPC results")
	}
	txHash, ok := result["transactionHash"].(string)
	if !ok {
		return errors.New("failed to parse tx hash from RPC results")
	}
	blockNum, ok := result["blockNumber"].(string)
	if !ok {
		nilBlockNum := result["blockNumber"]
		if nilBlockNum == nil {
			h.logger.Debug("skipping pending transaction", rpc.LogWithRequestID(ctx)...)
			return nil
		}

		return errors.New("failed to parse block number from RPC results")
	}

	var expiry time.Duration
	if h.fHelper.IsFinalizedHex(blockNum) {
		expiry = time.Hour
	} else {
		h.logger.Debug("not caching un-finalized tx receipt")
		return nil
	}

	cacheKey := txReceiptCacheKey(txHash)
	err = h.cacher.SetEx(cacheKey, parsed.Result, expiry)
	if err != nil {
		h.logger.Debug("post-processing failed while writing to cache", rpc.LogWithRequestID(ctx, "err", err)...)
		return err
	}
	h.logger.Debug("stored request in tx receipt cache", rpc.LogWithRequestID(ctx, "cache_key", cacheKey, "size", len(parsed.Result))...)
	return nil
}

func writeResponse(res http.ResponseWriter, id interface{}, data []byte) error {
	outJson := &rpc.JSONRPCRes{
		Jsonrpc: rpc.JSONRPC2,
		Id:      id,
		Result:  data,
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

func txReceiptCacheKey(hash string) string {
	return fmt.Sprintf("txreceipt:%s", hash)
}
