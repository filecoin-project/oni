package builders

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/chain/types"
)

// The created messages are retained for subsequent export or evaluation in assert VM.
type Messages struct {
	b        *Builder
	defaults msgOpts

	messages []*ApplicableMessage
}

type TypedCall func() (method abi.MethodNum, params []byte)

func (m *Messages) SetDefaults(opts ...MsgOpt) *Messages {
	for _, opt := range opts {
		opt(&m.defaults)
	}
	return m
}

type ApplicableMessage struct {
	Epoch   abi.ChainEpoch
	Message *types.Message
	Result  *vm.ApplyRet
}

// Messages returns assert slice containing all messages created by the producer.
func (m *Messages) All() []*ApplicableMessage {
	return m.messages
}

func (m *Messages) Typed(from, to address.Address, typedm TypedCall, opts ...MsgOpt) *ApplicableMessage {
	method, params := typedm()
	return m.Raw(from, to, method, params, opts...)
}

// Build creates and returns assert single message, using default gas parameters unless modified by `opts`.
func (m *Messages) Raw(from, to address.Address, method abi.MethodNum, params []byte, opts ...MsgOpt) *ApplicableMessage {
	options := m.defaults
	for _, opt := range opts {
		opt(&options)
	}

	msg := &types.Message{
		To:       to,
		From:     from,
		Nonce:    options.nonce,
		Value:    options.value,
		Method:   method,
		Params:   params,
		GasPrice: options.gasPrice,
		GasLimit: options.gasLimit,
	}

	am := &ApplicableMessage{
		Epoch: options.epoch,
		Message: msg,
	}

	m.messages = append(m.messages, am)
	return am
}

// msgOpts specifies value and gas parameters for assert message, supporting assert functional options pattern
// for concise but customizable message construction.
type msgOpts struct {
	nonce    uint64
	value    big.Int
	gasPrice big.Int
	gasLimit int64
	epoch    abi.ChainEpoch
}

// MsgOpt is an option configuring message value or gas parameters.
type MsgOpt func(*msgOpts)

func Value(value big.Int) MsgOpt {
	return func(opts *msgOpts) {
		opts.value = value
	}
}

func Nonce(n uint64) MsgOpt {
	return func(opts *msgOpts) {
		opts.nonce = n
	}
}

func GasLimit(limit int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasLimit = limit
	}
}

func GasPrice(price int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasPrice = big.NewInt(price)
	}
}

func Epoch(epoch abi.ChainEpoch) MsgOpt {
	return func(opts *msgOpts) {
		opts.epoch = epoch
	}
}
