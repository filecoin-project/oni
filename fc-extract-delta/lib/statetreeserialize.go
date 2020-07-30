package lib

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/state"
	format "github.com/ipfs/go-ipld-format"
	cid "github.com/ipfs/go-cid"
	car "github.com/ipld/go-car"
)

// SerializeStateTree provides the serialized car representation of a state tree
func SerializeStateTree(ctx context.Context, t *state.StateTree) ([]byte, error) {
	root, err := t.Flush(ctx)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err = car.WriteCar(ctx, stateTreeNodeGetter{t}, []cid.Cid{root}, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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

	if node, ok := out.(format.Node); ok {
		return node, nil
	}
	return nil, fmt.Errorf("can't deal with type %T", out)
}

func (s stateTreeNodeGetter) GetMany(ctx context.Context, cids []cid.Cid) <-chan *format.NodeOption {
	ch := make(chan *format.NodeOption, len(cids))
	go func() {
		defer close(ch)
		for _, c := range cids {
			n, e := s.Get(ctx, c)
			ch <- &format.NodeOption{n, e}
		}
	}()
	return ch
}
