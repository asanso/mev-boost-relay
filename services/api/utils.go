package api

import (
	"errors"

	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	gethCommon "github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/mev-boost-relay/common"
)

var (
	ErrBlockHashMismatch  = errors.New("blockHash mismatch")
	ErrParentHashMismatch = errors.New("parentHash mismatch")
)

func SanityCheckBuilderBlockSubmission(payload *common.BuilderSubmitBlockRequest) error {
	if payload.BlockHash() != payload.ExecutionPayloadBlockHash() {
		return ErrBlockHashMismatch
	}

	if payload.ParentHash() != payload.ExecutionPayloadParentHash() {
		return ErrParentHashMismatch
	}

	return nil
}

func checkBLSPublicKeyHex(pkHex string) error {
	var proposerPubkey types.PublicKey
	return proposerPubkey.UnmarshalText([]byte(pkHex))
}

func ComputeWithdrawalsRoot(withdrawals []*capella.Withdrawal) phase0.Root {
	withdrawalData := make([]*gethTypes.Withdrawal, len(withdrawals))
	for i, withdrawal := range withdrawals {
		withdrawalData[i] = &gethTypes.Withdrawal{
			Index:     uint64(withdrawal.Index),
			Validator: uint64(withdrawal.ValidatorIndex),
			Address:   gethCommon.Address(withdrawal.Address),
			Amount:    uint64(withdrawal.Amount),
		}
	}
	return phase0.Root(gethTypes.DeriveSha(gethTypes.Withdrawals(withdrawalData), trie.NewStackTrie(nil)))
}
