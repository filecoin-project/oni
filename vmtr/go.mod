module github.com/filecoin-project/oni/vmtr

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/lotus v0.4.3-0.20200729013254-5df0ee7935e0
	github.com/filecoin-project/sector-storage v0.0.0-20200727112136-9377cb376d25
	github.com/filecoin-project/specs-actors v0.8.1-0.20200728182452-1476088f645b
	github.com/ipfs/go-blockservice v0.1.4-0.20200624145336-a978cec6e834
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.1.2-0.20200626104915-0016c0b4b3e4
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/prometheus/procfs v0.1.3 // indirect
	github.com/urfave/cli/v2 v2.2.0
	go.opencensus.io v0.22.4 // indirect
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
