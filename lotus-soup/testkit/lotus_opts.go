package testkit

import (
	"fmt"

	"errors"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/gbrlsnchs/jwt/v3"
	"golang.org/x/xerrors"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	staticPrivateKey = []byte{9, 162, 18, 103, 226, 253, 122, 98, 170, 221, 129, 49, 89, 74, 187, 6, 54, 87, 56, 36, 249, 154, 254, 214, 210, 209, 0, 32, 196, 103, 171, 116}
)

func withGenesis(gb []byte) node.Option {
	return node.Override(new(modules.Genesis), modules.LoadGenesis(gb))
}

func withBootstrapper(ab []byte) node.Option {
	return node.Override(new(dtypes.BootstrapPeers),
		func() (dtypes.BootstrapPeers, error) {
			if ab == nil {
				return dtypes.BootstrapPeers{}, nil
			}

			a, err := ma.NewMultiaddrBytes(ab)
			if err != nil {
				return nil, err
			}
			ai, err := peer.AddrInfoFromP2pAddr(a)
			if err != nil {
				return nil, err
			}
			return dtypes.BootstrapPeers{*ai}, nil
		})
}

func withPubsubConfig(bootstrapper bool, pubsubTracer string) node.Option {
	return node.Override(new(*config.Pubsub), func() *config.Pubsub {
		return &config.Pubsub{
			Bootstrapper: bootstrapper,
			RemoteTracer: pubsubTracer,
		}
	})
}

func withListenAddress(ip string) node.Option {
	addrs := []string{fmt.Sprintf("/ip4/%s/tcp/0", ip)}
	return node.Override(node.StartListeningKey, lp2p.StartListening(addrs))
}

func withMinerListenAddress(ip string) node.Option {
	addrs := []string{fmt.Sprintf("/ip4/%s/tcp/0", ip)}
	return node.Override(node.StartListeningKey, lp2p.StartListening(addrs))
}

func withApiEndpoint(addr string) node.Option {
	return node.Override(node.SetApiEndpointKey, func(lr repo.LockedRepo) error {
		apima, err := ma.NewMultiaddr(addr)
		if err != nil {
			return err
		}
		return lr.SetAPIEndpoint(apima)
	})
}

type jwtPayload struct {
	Allow []auth.Permission
}

func withStaticAPISecret() node.Option {
	return node.Override(new(*dtypes.APIAlg), func(keystore types.KeyStore, lr repo.LockedRepo) (*dtypes.APIAlg, error) {
		key, err := keystore.Get(modules.JWTSecretName)

		if errors.Is(err, types.ErrKeyInfoNotFound) {
			key = types.KeyInfo{
				Type:       "jwt-hmac-secret",
				PrivateKey: staticPrivateKey,
			}

			if err := keystore.Put(modules.JWTSecretName, key); err != nil {
				return nil, xerrors.Errorf("writing API secret: %w", err)
			}

			// TODO: make this configurable
			p := jwtPayload{
				Allow: apistruct.AllPermissions,
			}

			cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.PrivateKey))
			if err != nil {
				return nil, err
			}

			if err := lr.SetAPIToken(cliToken); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, xerrors.Errorf("could not get JWT Token: %w", err)
		}

		return (*dtypes.APIAlg)(jwt.NewHS256(key.PrivateKey)), nil
	})
}
