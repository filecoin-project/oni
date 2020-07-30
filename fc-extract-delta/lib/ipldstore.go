package lib

import (
	"bytes"
	"context"
	"fmt"
	
	"github.com/filecoin-project/lotus/api"
	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
	localStore map[cid.Cid][]byte
}

func NewCache(ctx context.Context, node api.FullNode) *apiIpldStore {
	return &apiIpldStore{
		ctx,
		node,
		make(map[cid.Cid][]byte),
	}
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	var err error
	var item []byte
	var ok bool
	if item, ok = ht.localStore[c]; !ok {
		item, err = ht.api.ChainReadObj(ctx, c)
		if err != nil {
			return err
		}			
	}


	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(item)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("Object %#v does not implement CBORUnmarshaler", out)
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
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
