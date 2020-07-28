module github.com/filecoin-project/oni/vmtr

go 1.14

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/lotus v0.4.3-0.20200724113535-7410c057c6b2
	github.com/filecoin-project/sector-storage v0.0.0-20200723200950-ed2e57dde6df
	github.com/filecoin-project/specs-actors v0.8.1-0.20200724015154-3c690d9b7e1d
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-log/v2 v2.1.2-0.20200626104915-0016c0b4b3e4
	github.com/prometheus/procfs v0.1.3 // indirect
	go.opencensus.io v0.22.4 // indirect
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
