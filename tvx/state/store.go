package state

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

// ProxyingStores implements the ipld store where unknown items are fetched over the node API.
type ProxyingStores struct {
	CBORStore  cbor.IpldStore
	Datastore  ds.Batching
	Blockstore blockstore.Blockstore
}

type proxyingBlockstore struct {
	ctx context.Context
	api api.FullNode

	blockstore.Blockstore
}

func (pb *proxyingBlockstore) Get(cid cid.Cid) (blocks.Block, error) {
	if block, err := pb.Blockstore.Get(cid); err == nil {
		return block, err
	}

	// fmt.Printf("fetching cid via rpc: %v\n", cid)
	item, err := pb.api.ChainReadObj(pb.ctx, cid)
	if err != nil {
		return nil, err
	}
	block, err := blocks.NewBlockWithCid(item, cid)
	if err != nil {
		return nil, err
	}

	err = pb.Blockstore.Put(block)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// NewProxyingStore creates a new cache.
func NewProxyingStore(ctx context.Context, api api.FullNode) *ProxyingStores {
	ds := ds.NewMapDatastore()

	bs := &proxyingBlockstore{
		ctx:        ctx,
		api:        api,
		Blockstore: blockstore.NewBlockstore(ds),
	}

	cborstore := cbor.NewCborStore(bs)

	return &ProxyingStores{
		CBORStore:  cborstore,
		Datastore:  ds,
		Blockstore: bs,
	}
}
