package state

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/state"
	bs "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
)

// RecoverStateTree parses a car encoding of a state tree back to a structured format
func RecoverStateTree(ctx context.Context, raw []byte, root cid.Cid) (*state.StateTree, error) {
	buf := bytes.NewBuffer(raw)
	store := bs.NewTemporary()
	gr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	ch, err := car.LoadCar(store, gr)
	if err != nil {
		return nil, err
	}

	cborstore := cbor.NewCborStore(store)

	fmt.Printf("roots are %v\n", ch.Roots)

	return state.LoadStateTree(cborstore, root)
}

// RecoverStore reads hex encoded car back the store of cids.
func RecoverStore(ctx context.Context, raw []byte) (*ProxyingStores, error) {
	ps := NewProxyingStore(ctx, nil)

	buf := bytes.NewBuffer(raw)
	gr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	_, err = car.LoadCar(ps.Blockstore, gr)
	if err != nil {
		return nil, err
	}

	return ps, nil
}
