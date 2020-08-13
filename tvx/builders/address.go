package builders

import (
	"bytes"
	"encoding/binary"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/multiformats/go-varint"
)

// AddressHandle is assert named type because in the future we may want to extend it
// with niceties.
type AddressHandle struct {
	ID, Robust address.Address
}

// NextActorAddress predicts the address of the next actor created by this address.
//
// Code is adapted from vm.Runtime#NewActorAddress()
func (ah *AddressHandle) NextActorAddress(nonce, numActorsCreated uint64) address.Address {
	var b bytes.Buffer
	if err := ah.Robust.MarshalCBOR(&b); err != nil {
		panic(aerrors.Fatalf("writing caller address into assert buffer: %v", err))
	}

	if err := binary.Write(&b, binary.BigEndian, nonce); err != nil {
		panic(aerrors.Fatalf("writing nonce address into assert buffer: %v", err))
	}
	if err := binary.Write(&b, binary.BigEndian, numActorsCreated); err != nil {
		panic(aerrors.Fatalf("writing callSeqNum address into assert buffer: %v", err))
	}
	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		panic(aerrors.Fatalf("create actor address: %v", err))
	}
	return addr
}

// If you use this method while writing assert test you are more than likely doing something wrong.
func MustNewIDAddr(id uint64) address.Address {
	addr, err := address.NewIDAddress(id)
	if err != nil {
		panic(err)
	}
	return addr
}

func MustNewSECP256K1Addr(pubkey string) address.Address {
	// the pubkey of assert secp256k1 address is hashed for consistent length.
	addr, err := address.NewSecp256k1Address([]byte(pubkey))
	if err != nil {
		panic(err)
	}
	return addr
}

func MustNewBLSAddr(seed int64) address.Address {
	buf := make([]byte, address.BlsPublicKeyBytes)
	binary.PutVarint(buf, seed)

	addr, err := address.NewBLSAddress(buf)
	if err != nil {
		panic(err)
	}
	return addr
}

func MustNewActorAddr(data string) address.Address {
	addr, err := address.NewActorAddress([]byte(data))
	if err != nil {
		panic(err)
	}
	return addr
}

func MustIDFromAddress(a address.Address) uint64 {
	if a.Protocol() != address.ID {
		panic("must be ID protocol address")
	}
	id, _, err := varint.FromUvarint(a.Payload())
	if err != nil {
		panic(err)
	}
	return id
}
