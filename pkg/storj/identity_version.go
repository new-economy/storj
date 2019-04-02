// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package storj

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"strconv"
	"strings"

	"storj.io/storj/pkg/peertls/extensions"
	"storj.io/storj/pkg/pkcrypto"
)

const (
	// V1 represents identity version 1
	// NB: identities created before identity versioning will be recognized as V1.
	V1 = IDVersionNumber(iota + 1)
	// V2 represents identity version 2
	V2
)

var (
	// IDVersions is a map of all identity versions
	IDVersions = map[IDVersionNumber]IDVersion{
		/* V1 breaking change:
		+ removed support for difficulties < 9
		*/
		V1: {
			Number:        V1,
			NewPrivateKey: pkcrypto.GeneratePrivateKey,
		},
		/* V2 changes:
		+ add version support
		+ change elliptic curve to non-NIST
		*/
		V2: {
			Number:        V2,
			NewPrivateKey: pkcrypto.GeneratePrivateKey,
		},
	}

	// IDVersionHandler compares the identity version of the remote peers
	// certificate chain to the extension options passed to the factory.
	IDVersionHandler = extensions.NewHandlerFactory(
		&extensions.IdentityVersionExtID, idVersionHandler,
	)
)

// IDVersionNumber is the number of an identity version.
type IDVersionNumber uint8

// IDVersion holds fields that are used to distinguish different identity
// versions from one another; used in identity generation.
type IDVersion struct {
	Number        IDVersionNumber
	NewPrivateKey func() (crypto.PrivateKey, error)
}

func init() {
	extensions.AllHandlers.Register(IDVersionHandler)
}

// GetIDVersion looks up the given version number in the map of registered
// versions, returning an error if none is found.
func GetIDVersion(number IDVersionNumber) (IDVersion, error) {
	if number == 0 {
		return LatestIDVersion(), nil
	}

	version, ok := IDVersions[number]
	if !ok {
		return IDVersion{}, ErrVersion.New("unknown version")
	}

	return version, nil
}

// LatestIDVersion returns the last IDVersion registered.
func LatestIDVersion() IDVersion {
	return IDVersions[IDVersionNumber(len(IDVersions))]
}

// IDVersionFromCert parsed the IDVersion from the passed certificate's IDVersion extension.
func IDVersionFromCert(cert *x509.Certificate) (IDVersion, error) {
	for _, ext := range cert.Extensions {
		if extensions.IdentityVersionExtID.Equal(ext.Id) {
			return GetIDVersion(IDVersionNumber(ext.Value[0]))
		}
	}

	// NB: for backward-compatibility with V1 certificate generation, V1 is used
	// when no version extension exists.
	// TODO(beta maybe?): Error here instead; we should drop support for
	//  certificates without a version extension.
	//
	// return IDVersion{}, ErrVersion.New("certificate doesn't contain an identity version extension")
	return IDVersions[V1], nil
}

// IDVersionInVersions returns an error if the given version is in the given string of version(s)/range(s).
func IDVersionInVersions(versionNumber IDVersionNumber, versionsStr string) error {
	switch versionsStr {
	case "":
		return ErrVersion.New("no allowed peer identity versions specified")
	case "latest":
		if versionNumber == LatestIDVersion().Number {
			return nil
		}
	default:
		versionRanges := strings.Split(versionsStr, ",")
		for _, versionRange := range versionRanges {
			if strings.Contains(versionRange, "-") {
				versionLimits := strings.Split(versionRange, "-")
				if len(versionLimits) != 2 {
					return ErrVersion.New("malformed PeerIDVersions string: %s", versionsStr)
				}

				begin, err := strconv.Atoi(versionLimits[0])
				if err != nil {
					return ErrVersion.Wrap(err)
				}

				end, err := strconv.Atoi(versionLimits[1])
				if err != nil {
					return ErrVersion.Wrap(err)
				}

				for i := begin; i <= end; i++ {
					if versionNumber == IDVersionNumber(i) {
						return nil
					}
				}
			} else {
				versionInt, err := strconv.Atoi(versionRange)
				if err != nil {
					return ErrVersion.Wrap(err)
				}
				if versionNumber == IDVersionNumber(versionInt) {
					return nil
				}
			}
		}
	}
	return ErrVersion.New("version %d not in versions %s", versionNumber, versionsStr)
}

func idVersionHandler(opts *extensions.Options) extensions.HandlerFunc {
	return func(ext pkix.Extension, chain [][]*x509.Certificate) error {
		return IDVersionInVersions(IDVersionNumber(ext.Value[0]), opts.PeerIDVersions)
	}
}
