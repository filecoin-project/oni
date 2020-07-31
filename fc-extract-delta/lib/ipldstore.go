package lib

import (
	"bytes"
	"context"
	"fmt"
	
	"github.com/filecoin-project/lotus/api"
	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	cbor "github.com/ipfs/go-ipld-cbor"
)

// CacheStore implements the ipld store where unknown items are fetched over the node API.
type CacheStore struct {
	ctx context.Context
	api api.FullNode
	localStore map[cid.Cid][]byte
}

// NewCache creates a new cache.
func NewCache(ctx context.Context, node api.FullNode) *CacheStore {
	return &CacheStore{
		ctx,
		node,
		make(map[cid.Cid][]byte),
	}
}

// Context provides the context the store operates within.
func (ht *CacheStore) Context() context.Context {
	return ht.ctx
}

type nodeset interface {
	SetRaw(ctx context.Context, k string, raw []byte) error
}

// Get populates `out` from `c`
func (ht *CacheStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	var err error
	var item []byte
	var ok bool
	if item, ok = ht.localStore[c]; !ok {
		item, err = ht.api.ChainReadObj(ctx, c)
		if err != nil {
			if c.Prefix().Codec != cid.DagCBOR {
				return nil
			}
			return fmt.Errorf("Failed for cid %v: %w", c, err)
		}			
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
func (ht *CacheStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	cm, ok := v.(cbg.CBORMarshaler)
	if ok {
		buf := bytes.Buffer{}
		if err := cm.MarshalCBOR(&buf); err != nil {
			return cid.Cid{}, err
		}
		idgen := cid.V1Builder{}
		val := buf.Bytes()
		lcid, _ := idgen.Sum(val)
		ht.localStore[lcid] = val
		return lcid, nil
	}	
	return cid.Cid{}, fmt.Errorf("Object doesnt impl cbormarshaler")
}
