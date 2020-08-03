package state

import (
	"bytes"
	"context"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/ipfs/go-cid"
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
	if err = car.WriteCar(ctx, stateTreeNodeGetter{t}, []cid.Cid{root}, &buf); err != nil {
		return nil, cid.Undef, err
	}

	return buf.Bytes(), root, nil
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
