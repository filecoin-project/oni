module github.com/filecoin-project/oni/lotus-soup

go 1.14

require (
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/davecgh/go-spew v1.1.1
	github.com/drand/drand v1.0.3-0.20200714175734-29705eaf09d4
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-bitfield v0.0.4-0.20200703174658-f4a5758051a1
	github.com/filecoin-project/go-fil-markets v0.4.1-0.20200715201050-c141144ea312
	github.com/filecoin-project/go-jsonrpc v0.1.1-0.20200602181149-522144ab4e24
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.4.2-0.20200716234857-389d148a60c1
	github.com/filecoin-project/specs-actors v0.7.2
	github.com/filecoin-project/storage-fsm v0.0.0-20200715191202-7e92e888bf41
	github.com/gorilla/mux v1.7.4
	github.com/influxdata/influxdb v1.8.0 // indirect
	github.com/ipfs/go-cid v0.0.6
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.1.2-0.20200626104915-0016c0b4b3e4
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/kpacha/opencensus-influxdb v0.0.0-20181102202715-663e2683a27c
	github.com/libp2p/go-libp2p v0.10.0
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/libp2p/go-libp2p-pubsub-tracer v0.0.0-20200626141350-e730b32bf1e6
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/testground/sdk-go v0.2.3-0.20200706132230-6a65ddac2d8c
	go.opencensus.io v0.22.4
)

// This will work in all build modes: docker:go, exec:go, and local go build.
// On docker:go and exec:go, it maps to /extra/filecoin-ffi, as it's picked up
// as an "extra source" in the manifest.
replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
