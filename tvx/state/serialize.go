package state

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/state"
	bs "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"
	mh "github.com/multiformats/go-multihash"
)

// SerializeStateTree provides the serialized car representation of a state tree
func SerializeStateTree(ctx context.Context, t *state.StateTree) ([]byte, cid.Cid, error) {
	root, err := t.Flush(ctx)
	if err != nil {
		return nil, cid.Undef, err
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if err = car.WriteCar(ctx, stateTreeNodeGetter{t}, []cid.Cid{root}, gw); err != nil {
		return nil, cid.Undef, err
	}
	if err = gw.Flush(); err != nil {
		return nil, cid.Undef, err
	}
	if err = gw.Close(); err != nil {
		return nil, cid.Undef, err
	}

	return buf.Bytes(), root, nil
}

// RecoverStateTree parses a car encoding of a state tree back to a structured format
func RecoverStateTree(ctx context.Context, raw []byte) (*state.StateTree, error) {
	buf := bytes.NewBuffer(raw)
	store := bs.NewTemporary()
	gr, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	ch, err := car.LoadCar(store, gr)
	if err != nil {
		return nil, err
	}
	if len(ch.Roots) != 1 {
		return nil, fmt.Errorf("car should have 1 root, has %d", len(ch.Roots))
	}
	ipldStore := cbor.NewCborStore(store)

	fmt.Printf("root is %s\n", ch.Roots[0])

	nd, err := hamt.LoadNode(ctx, ipldStore, ch.Roots[0], hamt.UseTreeBitWidth(5))
	if err != nil {
		return nil, err
	}
	if err := nd.ForEach(ctx, func(k string, val interface{}) error {
		fmt.Printf("hampt %s\n", k)
		return nil
	}); err != nil {
		return nil, err
	}

	return state.LoadStateTree(ipldStore, ch.Roots[0])
}

// stateTreeNodeGetter implements format.NodeGetter over a state tree
type stateTreeNodeGetter struct {
	*state.StateTree
}

func (s stateTreeNodeGetter) Get(ctx context.Context, c cid.Cid) (format.Node, error) {
	var out interface{}
	err := s.Store.Get(ctx, c, &out)
	if err != nil {
		return nil, err
	}

	return cbor.WrapObject(out, mh.SHA2_256, 32)
}

func (s stateTreeNodeGetter) GetMany(ctx context.Context, cids []cid.Cid) <-chan *format.NodeOption {
	ch := make(chan *format.NodeOption, len(cids))
	go func() {
		defer close(ch)
		for _, c := range cids {
			n, e := s.Get(ctx, c)
			ch <- &format.NodeOption{Node: n, Err: e}
		}
	}()
	return ch
}
