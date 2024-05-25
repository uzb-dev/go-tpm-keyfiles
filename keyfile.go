package keyfile

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	encasn1 "encoding/asn1"
	"fmt"
	"math/big"

	"github.com/google/go-tpm/tpm2"
)

type TPMPolicy struct {
	commandCode   int
	commandPolicy []byte
}

type TPMAuthPolicy struct {
	name   string
	policy []*TPMPolicy
}

type TPMKey struct {
	keytype     encasn1.ObjectIdentifier
	emptyAuth   bool
	policy      []*TPMPolicy
	secret      []byte
	authPolicy  []*TPMAuthPolicy
	description string
	Parent      tpm2.TPMHandle
	Pubkey      tpm2.TPM2BPublic
	Privkey     tpm2.TPM2BPrivate
}

func (t *TPMKey) HasSinger() bool {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		panic("can't serialize public key")
	}
	return pub.ObjectAttributes.SignEncrypt
}

func (t *TPMKey) HasAuth() bool {
	return !t.emptyAuth
}

func (t *TPMKey) KeyAlgo() tpm2.TPMAlgID {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		panic("can't serialize public key")
	}
	return pub.Type
}

func (t *TPMKey) SetDescription(s string) {
	t.description = s
}

func (t *TPMKey) Description() string {
	return t.description
}

func (t *TPMKey) Bytes() []byte {
	var b bytes.Buffer
	if err := Encode(&b, t); err != nil {
		return nil
	}
	return b.Bytes()
}

func (t *TPMKey) ecdsaPubKey() (*ecdsa.PublicKey, error) {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		return nil, fmt.Errorf("can't serialize public key contents")
	}
	ecc, err := pub.Unique.ECC()
	if err != nil {
		return nil, err
	}

	eccdeets, err := pub.Parameters.ECCDetail()
	if err != nil {
		return nil, err
	}

	var ecdsaKey *ecdsa.PublicKey

	switch eccdeets.CurveID {
	case tpm2.TPMECCNistP256:
		ecdsaKey = &ecdsa.PublicKey{Curve: elliptic.P256(),
			X: big.NewInt(0).SetBytes(ecc.X.Buffer),
			Y: big.NewInt(0).SetBytes(ecc.Y.Buffer),
		}
	case tpm2.TPMECCNistP384:
		ecdsaKey = &ecdsa.PublicKey{Curve: elliptic.P384(),
			X: big.NewInt(0).SetBytes(ecc.X.Buffer),
			Y: big.NewInt(0).SetBytes(ecc.Y.Buffer),
		}
	case tpm2.TPMECCNistP521:
		ecdsaKey = &ecdsa.PublicKey{Curve: elliptic.P521(),
			X: big.NewInt(0).SetBytes(ecc.X.Buffer),
			Y: big.NewInt(0).SetBytes(ecc.Y.Buffer),
		}
	}

	return ecdsaKey, nil
}

func (t *TPMKey) rsaPubKey() (*rsa.PublicKey, error) {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		return nil, fmt.Errorf("can't serialize public key contents")
	}
	rsaDetail, err := pub.Parameters.RSADetail()
	if err != nil {
		return nil, fmt.Errorf("failed getting rsa details: %v", err)
	}
	rsaUnique, err := pub.Unique.RSA()
	if err != nil {
		return nil, fmt.Errorf("failed getting unique rsa: %v", err)
	}

	return tpm2.RSAPub(rsaDetail, rsaUnique)
}

// PublicKey returns the ecdsa.Publickey or rsa.Publickey of the TPMKey
func (t *TPMKey) PublicKey() (any, error) {
	switch t.KeyAlgo() {
	case tpm2.TPMAlgECC:
		return t.ecdsaPubKey()
	case tpm2.TPMAlgRSA:
		return t.rsaPubKey()
	}
	return nil, fmt.Errorf("no public key")
}

func NewLoadableKey(public tpm2.TPM2BPublic, private tpm2.TPM2BPrivate, parent tpm2.TPMHandle, emptyAuth bool) (*TPMKey, error) {
	var key TPMKey
	key.keytype = OIDLoadableKey
	key.emptyAuth = emptyAuth

	key.Pubkey = public
	key.Privkey = private

	key.Parent = parent

	return &key, nil
}