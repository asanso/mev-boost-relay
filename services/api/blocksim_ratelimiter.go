package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/go-utils/jsonrpc"
	"github.com/flashbots/mev-boost-relay/common"
)

var (
	ErrRequestClosed    = errors.New("request context closed")
	ErrSimulationFailed = errors.New("simulation failed")
	ErrChainProgressed  = errors.New("chain has progressed past payload slot")
	ErrNoPayload        = errors.New("no payload")

	maxConcurrentBlocks = int64(cli.GetEnvInt("BLOCKSIM_MAX_CONCURRENT", 4)) // 0 for no maximum
	simRequestTimeout   = time.Duration(cli.GetEnvInt("BLOCKSIM_TIMEOUT_MS", 3000)) * time.Millisecond
)

type cxCtx struct {
	ctx    context.Context //nolint:containedctx
	cancel context.CancelFunc
}

type BlockSimulationRateLimiter struct {
	cv          *sync.Cond
	counter     int64
	blockSimURL string
	client      http.Client

	headSlot atomic.Uint64

	requestsInFlight     map[*cxCtx]uint64
	requestsInFlightLock sync.Mutex
}

func NewBlockSimulationRateLimiter(blockSimURL string) *BlockSimulationRateLimiter {
	return &BlockSimulationRateLimiter{
		cv:          sync.NewCond(&sync.Mutex{}),
		counter:     0,
		headSlot:    atomic.Uint64{},
		blockSimURL: blockSimURL,
		client: http.Client{ //nolint:exhaustruct
			Timeout: simRequestTimeout,
		},

		requestsInFlight:     make(map[*cxCtx]uint64),
		requestsInFlightLock: sync.Mutex{},
	}
}

func (b *BlockSimulationRateLimiter) updateHeadSlot(headSlot uint64) {
	prevHeadSlot := b.headSlot.Load()
	if headSlot <= prevHeadSlot {
		return
	}
	b.headSlot.Store(headSlot)

	// now delete all requests that are in-flight but for slots that are now in the past
	b.requestsInFlightLock.Lock()
	defer b.requestsInFlightLock.Unlock()
	for cxCtx, payloadSlot := range b.requestsInFlight {
		if headSlot >= payloadSlot {
			if cxCtx.ctx.Err() == nil {
				cxCtx.cancel()
			}
			delete(b.requestsInFlight, cxCtx)
		}
	}
}

func (b *BlockSimulationRateLimiter) send(ctx context.Context, payload *common.BuilderBlockValidationRequest, isHighPrio, cancelOnChainProgress bool) error {
	b.cv.L.Lock()
	cnt := atomic.AddInt64(&b.counter, 1)
	if maxConcurrentBlocks > 0 && cnt > maxConcurrentBlocks {
		b.cv.Wait()
	}
	b.cv.L.Unlock()

	defer func() {
		b.cv.L.Lock()
		atomic.AddInt64(&b.counter, -1)
		b.cv.Signal()
		b.cv.L.Unlock()
	}()

	if err := ctx.Err(); err != nil {
		return ErrRequestClosed
	}

	// request may be allowed to get cancelled after chain progresses
	if cancelOnChainProgress {
		if b.headSlot.Load() >= payload.Slot() {
			return ErrChainProgressed
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		b.requestsInFlightLock.Lock()
		b.requestsInFlight[&cxCtx{
			ctx:    ctx,
			cancel: cancel,
		}] = payload.Slot()
		b.requestsInFlightLock.Unlock()
	}

	var simReq *jsonrpc.JSONRPCRequest
	var simResp *jsonrpc.JSONRPCResponse
	var err error
	if payload.Bellatrix != nil {
		simReq = jsonrpc.NewJSONRPCRequest("1", "flashbots_validateBuilderSubmissionV1", payload)
	} else if payload.Capella != nil {
		simReq = jsonrpc.NewJSONRPCRequest("1", "flashbots_validateBuilderSubmissionV2", payload)
	} else {
		return ErrNoPayload
	}

	simResp, err = SendJSONRPCRequest(&b.client, ctx, *simReq, b.blockSimURL, isHighPrio)
	if err != nil {
		return err
	} else if simResp.Error != nil {
		return fmt.Errorf("%w: %s", ErrSimulationFailed, simResp.Error.Message)
	}
	return nil
}

// currentCounter returns the number of waiting and active requests
func (b *BlockSimulationRateLimiter) currentCounter() int64 {
	return atomic.LoadInt64(&b.counter)
}

// SendJSONRPCRequest sends the request to URL and returns the general JsonRpcResponse, or an error (note: not the JSONRPCError)
func SendJSONRPCRequest(client *http.Client, ctx context.Context, req jsonrpc.JSONRPCRequest, url string, isHighPrio bool) (res *jsonrpc.JSONRPCResponse, err error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	// set request headers
	httpReq.Header.Add("Content-Type", "application/json")
	if isHighPrio {
		httpReq.Header.Add("X-High-Priority", "true")
	}

	// execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res = new(jsonrpc.JSONRPCResponse)
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, err
	}

	return res, nil
}
