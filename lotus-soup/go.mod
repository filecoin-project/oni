module github.com/filecoin-project/oni/lotus-soup

go 1.14

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.3.1-0.20200518172415-1ed618334471
	github.com/filecoin-project/specs-actors v0.6.2-0.20200617175406-de392ca14121
	github.com/ipfs/go-datastore v0.4.4
	github.com/libp2p/go-libp2p-core v0.5.7
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/testground/sdk-go v0.2.3-0.20200617132925-2e4d69f9ba38
)

replace github.com/filecoin-project/filecoin-ffi => ../lotus/extern/filecoin-ffi

replace github.com/filecoin-project/lotus => ../lotus
