package state

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// ReadThroughStore implements the ipld store where unknown items are fetched over the node API.
type ReadThroughStore struct {
	ctx   context.Context
	api   api.FullNode
	local map[cid.Cid][]byte
}

// NewReadThroughStore creates a new cache.
func NewReadThroughStore(ctx context.Context, node api.FullNode) *ReadThroughStore {
	return &ReadThroughStore{
		ctx,
		node,
		make(map[cid.Cid][]byte),
	}
}

// Context provides the context the store operates within.
func (s *ReadThroughStore) Context() context.Context {
	return s.ctx
}

// for dealing with HAMTs.
type nodeset interface {
	SetRaw(ctx context.Context, k string, raw []byte) error
}

// Get populates `out` from `c`
func (s *ReadThroughStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	var err error
	var item []byte
	var ok bool
	if item, ok = s.local[c]; !ok {
		fmt.Fprintf(os.Stderr, "fetching cid via rpc: %v\n", c)
		item, err = s.api.ChainReadObj(ctx, c)
		if err != nil {
			if c.Prefix().Codec != cid.DagCBOR {
				return nil
			}
			return fmt.Errorf("Failed for cid %v: %w", c, err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "cid cached locally: %v\n", c)
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(item)); err != nil {
			return err
		}
		return nil
	}

	hn, ok := out.(nodeset)
	if ok {
		if err := hn.SetRaw(ctx, c.String(), item); err != nil {
			return err
		}
		return nil
	}

	return cbor.DecodeInto(item, out)
}

// Put generates a cid for and stores `v`
func (s *ReadThroughStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	cm, ok := v.(cbg.CBORMarshaler)
	if ok {
		buf := bytes.Buffer{}
		if err := cm.MarshalCBOR(&buf); err != nil {
			return cid.Cid{}, err
		}
		idgen := cid.V1Builder{}
		val := buf.Bytes()
		lcid, _ := idgen.Sum(val)
		s.local[lcid] = val
		return lcid, nil
	}
	return cid.Cid{}, fmt.Errorf("Object doesnt impl cbormarshaler")
}
