package drivers

import (
	"context"
	"fmt"
	"io"

	"github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipfs/go-blockservice"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"

	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
)

const (
	totalFilecoin     = 2_000_000_000
	filecoinPrecision = 1_000_000_000_000_000_000
)

var (
	TotalNetworkBalance = big_spec.Mul(big_spec.NewInt(totalFilecoin), big_spec.NewInt(filecoinPrecision))
	EmptyReturnValue    = []byte{}
)

// Actor is an abstraction over the actor states stored in the root of the state tree.
type Actor interface {
	Code() cid.Cid
	Head() cid.Cid
	CallSeqNum() uint64
	Balance() big.Int
}

type contextStore struct {
	cbor.IpldStore
	ctx context.Context
}

func (s *contextStore) Context() context.Context {
	return s.ctx
}

func serialise(bs blockstore.Blockstore, c cid.Cid, w io.Writer) error {
	ctx := context.Background()

	offl := offline.Exchange(bs)
	blkserv := blockservice.New(bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{c}, w, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	return nil
}

func walker(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		//spew.Dump(link)
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}
