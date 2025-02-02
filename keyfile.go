package keyfile

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	encasn1 "encoding/asn1"
	"fmt"
	"math/big"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
)

type TPMPolicy struct {
	CommandCode   int
	CommandPolicy []byte
}

type TPMAuthPolicy struct {
	Name   string
	Policy []*TPMPolicy
}

type TPMKey struct {
	Keytype     encasn1.ObjectIdentifier
	EmptyAuth   bool
	Policy      []*TPMPolicy
	Secret      tpm2.TPM2BEncryptedSecret
	AuthPolicy  []*TPMAuthPolicy
	Description string
	Parent      tpm2.TPMHandle
	Pubkey      tpm2.TPM2BPublic
	Privkey     tpm2.TPM2BPrivate
	userAuth    []byte // Internal detail
}

func NewTPMKey(oid encasn1.ObjectIdentifier, pubkey tpm2.TPM2BPublic, privkey tpm2.TPM2BPrivate, fn ...TPMKeyOption) *TPMKey {
	var key TPMKey

	// Set defaults
	key.AddOptions(
		WithKeytype(oid),
		// We always start of with assuming this key shouldn't have an auth
		WithUserAuth([]byte(nil)),
		// Start out with setting the Owner as parent
		WithParent(tpm2.TPMRHOwner),
		WithPubkey(pubkey),
		WithPrivkey(privkey),
	)

	key.AddOptions(fn...)
	return &key
}

func (t *TPMKey) AddOptions(fn ...TPMKeyOption) {
	// Run over TPMKeyFn
	for _, f := range fn {
		f(t)
	}
}

func (t *TPMKey) HasSinger() bool {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		panic("can't serialize public key")
	}
	return pub.ObjectAttributes.SignEncrypt
}

func (t *TPMKey) HasAuth() bool {
	return !t.EmptyAuth
}

func (t *TPMKey) KeyAlgo() tpm2.TPMAlgID {
	pub, err := t.Pubkey.Contents()
	if err != nil {
		panic("can't serialize public key")
	}
	return pub.Type
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

// Wraps TPMSigner with some sane defaults
// Use NewTPMSigner if you need more control of the parameters
func (t *TPMKey) Signer(tpm transport.TPMCloser, ownerAuth, auth []byte) (crypto.Signer, error) {
	if !t.HasSinger() {
		// TODO: Implement support for signing with Decrypt operations
		return nil, fmt.Errorf("does not have sign/encrypt attribute set")
	}
	return NewTPMKeySigner(
		t,
		func() ([]byte, error) { return ownerAuth, nil },
		func() transport.TPMCloser { return tpm },
		func(_ *TPMKey) ([]byte, error) { return auth, nil },
	), nil
}

func (t *TPMKey) Verify(alg crypto.Hash, hashed []byte, sig []byte) (bool, error) {
	pubkey, err := t.PublicKey()
	if err != nil {
		return false, fmt.Errorf("failed getting pubkey: %v", err)
	}
	switch pk := pubkey.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pk, hashed[:], sig) {
			return false, fmt.Errorf("invalid signature")
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pk, alg, hashed[:], sig); err != nil {
			return false, fmt.Errorf("signature verification failed: %v", err)
		}
	}
	return true, nil
}
