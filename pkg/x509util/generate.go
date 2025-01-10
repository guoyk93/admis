package x509util

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// CreateCertificatePEM create certificate with PEM output
func CreateCertificatePEM(template, parent *x509.Certificate, pub any, priv any) (crt *x509.Certificate, crtPEM []byte, err error) {
	var raw []byte
	if raw, err = x509.CreateCertificate(rand.Reader, template, parent, pub, priv); err != nil {
		return
	}
	if crt, err = x509.ParseCertificate(raw); err != nil {
		return
	}
	crtPEM = encodePEM(raw, PEMTypeCertificate)
	return
}

// GeneratePrivateKeyPEM generate a private key with PEM output in PKCS8 format
func GeneratePrivateKeyPEM(alg x509.PublicKeyAlgorithm) (key crypto.Signer, keyPEM []byte, err error) {
	switch alg {
	case x509.RSA:
		if key, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			return
		}
	case x509.ECDSA:
		if key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader); err != nil {
			return
		}
	case x509.Ed25519:
		if _, key, err = ed25519.GenerateKey(rand.Reader); err != nil {
			return
		}
	default:
		err = fmt.Errorf("gracex509.GeneratePrivateKeyPEM(): unknown PublicKeyAlgorithm: %02x", alg)
		return
	}

	var raw []byte
	if raw, err = x509.MarshalPKCS8PrivateKey(key); err != nil {
		return
	}

	keyPEM = encodePEM(raw, PEMTypePrivateKey)
	return
}

const (
	DefaultCountry            = "CN"
	DefaultOrganization       = "yankeguo.github.io"
	DefaultExpires            = time.Hour * 24 * 365 * 30
	DefaultPublicKeyAlgorithm = x509.RSA
)

// GenerateOptions options for certificate generation
type GenerateOptions struct {
	// Parent certificate of parent, required when IsCA is false
	Parent PEMPair
	// IsCA whether to generate a IsCA certificate
	IsCA bool
	// PublicKeyAlgorithm x509 public key algorithm, default to RSA
	PublicKeyAlgorithm x509.PublicKeyAlgorithm
	// Names certificate names, tailing names will be used as DNSNames
	Names []string
	// Country certificate country
	Country string
	// Organization certificate organization
	Organization string
	// Expires certificate duration
	Expires time.Duration
}

// Generate easily generate a certificate with RSA private key
// if both ParentCrtPEM and CAKeyPem is missing, will generate a new IsCA
func Generate(opts GenerateOptions) (res PEMPair, err error) {
	// check options
	if len(opts.Names) < 1 {
		err = errors.New("gracex509.Generate: opts.Names missing")
		return
	}
	if opts.PublicKeyAlgorithm == 0 {
		opts.PublicKeyAlgorithm = DefaultPublicKeyAlgorithm
	}
	if opts.Country == "" {
		opts.Country = DefaultCountry
	}
	if opts.Organization == "" {
		opts.Organization = DefaultOrganization
	}
	if opts.Expires <= 0 {
		opts.Expires = DefaultExpires
	}
	if opts.Parent.IsZero() && !opts.IsCA {
		err = errors.New("gracex509.Generate: both opts.IsCA is false and opts.Parent is missing")
		return
	}

	var resKey crypto.Signer
	if resKey, res.Key, err = GeneratePrivateKeyPEM(opts.PublicKeyAlgorithm); err != nil {
		return
	}
	resKeyPub := resKey.Public()

	var (
		notBefore = time.Now().Add(-time.Second * 10)
		notAfter  = notBefore.Add(opts.Expires)
	)

	var template *x509.Certificate

	if opts.Parent.IsZero() {
		// root-ca
		template = &x509.Certificate{
			SerialNumber: big.NewInt(notBefore.UnixMilli()),
			Subject: pkix.Name{
				Country:      []string{opts.Country},
				Organization: []string{opts.Organization},
				CommonName:   opts.Names[0],
			},
			DNSNames:              opts.Names[1:],
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLen:            2,
		}

		// create crt
		if _, res.Crt, err = CreateCertificatePEM(template, template, resKeyPub, resKey); err != nil {
			return
		}

	} else {
		// leaf or middle-ca certificate
		var (
			parentCrt *x509.Certificate
			parentKey any
		)
		if parentCrt, parentKey, err = opts.Parent.Decode(); err != nil {
			err = errors.New("grace509.Generate(): failed to decode opts.Parent: " + err.Error())
			return
		}
		if !parentCrt.IsCA {
			err = errors.New("grace509.Generate(): opts.Parent is not a CA")
			return
		}
		if opts.IsCA {
			// middle-ca
			template = &x509.Certificate{
				SerialNumber: big.NewInt(notBefore.UnixMilli()),
				Subject: pkix.Name{
					Country:      []string{opts.Country},
					Organization: []string{opts.Organization},
					CommonName:   opts.Names[0],
				},
				DNSNames:              opts.Names[1:],
				NotBefore:             notBefore,
				NotAfter:              notAfter,
				KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
				ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				BasicConstraintsValid: true,
				IsCA:                  true,
				MaxPathLen:            1,
			}
		} else {
			// leaf
			template = &x509.Certificate{
				SerialNumber: big.NewInt(notBefore.UnixMilli()),
				Subject: pkix.Name{
					Country:      []string{opts.Country},
					Organization: []string{opts.Organization},
					CommonName:   opts.Names[0],
				},
				DNSNames:    opts.Names[1:],
				NotBefore:   notBefore,
				NotAfter:    notAfter,
				KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}
		}

		// create crt
		if _, res.Crt, err = CreateCertificatePEM(template, parentCrt, resKeyPub, parentKey); err != nil {
			return
		}
	}

	return
}
