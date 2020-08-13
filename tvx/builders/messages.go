package builders

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/chain/types"
)

// TypedCall represents a call to a known built-in actor kind.
type TypedCall func() (method abi.MethodNum, params []byte)

// Messages accumulates the messages to be executed within the test vector.
type Messages struct {
	b        *Builder
	defaults msgOpts

	messages []*ApplicableMessage
}

// SetDefaults sets default options for all messages.
func (m *Messages) SetDefaults(opts ...MsgOpt) *Messages {
	for _, opt := range opts {
		opt(&m.defaults)
	}
	return m
}

// ApplicableMessage represents a message to be applied on the test vector.
type ApplicableMessage struct {
	Epoch   abi.ChainEpoch
	Message *types.Message
	Result  *vm.ApplyRet
}

func (m *Messages) Sugar() *sugarMsg {
	return &sugarMsg{m}
}

// All returns all ApplicableMessages that have been accumulated, in the same
// order they were added.
func (m *Messages) All() []*ApplicableMessage {
	return m.messages
}

// Typed adds a typed call to this message accumulator.
func (m *Messages) Typed(from, to address.Address, typedm TypedCall, opts ...MsgOpt) *ApplicableMessage {
	method, params := typedm()
	return m.Raw(from, to, method, params, opts...)
}

// Raw adds a raw message to this message accumulator.
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
		Epoch:   options.epoch,
		Message: msg,
	}

	m.messages = append(m.messages, am)
	return am
}

type msgOpts struct {
	nonce    uint64
	value    big.Int
	gasPrice big.Int
	gasLimit int64
	epoch    abi.ChainEpoch
}

// MsgOpt is an option configuring message value, gas parameters, execution
// epoch, and other elements.
type MsgOpt func(*msgOpts)

// Value sets a value on a message.
func Value(value big.Int) MsgOpt {
	return func(opts *msgOpts) {
		opts.value = value
	}
}

// Nonce sets the nonce of a message.
func Nonce(n uint64) MsgOpt {
	return func(opts *msgOpts) {
		opts.nonce = n
	}
}

// GasLimit sets the gas limit of a message.
func GasLimit(limit int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasLimit = limit
	}
}

// GasPrice sets the gas price of a message.
func GasPrice(price int64) MsgOpt {
	return func(opts *msgOpts) {
		opts.gasPrice = big.NewInt(price)
	}
}

// Epoch sets the epoch in which a message is to be executed.
func Epoch(epoch abi.ChainEpoch) MsgOpt {
	return func(opts *msgOpts) {
		opts.epoch = epoch
	}
}
