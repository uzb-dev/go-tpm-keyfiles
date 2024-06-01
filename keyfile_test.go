package keyfile

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/google/go-tpm/tpm2"
)

func mustOpen(s string) []byte {
	b, err := os.ReadFile(s)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func TestParse(t *testing.T) {
	for n, tt := range []struct {
		name string
		f    []byte
	}{
		{
			name: "plain rsa key",
			f:    mustOpen("./testdata/rsa-key.tpm"),
		},
		{
			name: "plain rsa key with password",
			f:    mustOpen("./testdata/rsa-tpm-password.tpm"),
		},
		{
			name: "p256 with authvalue",
			f:    mustOpen("./testdata/p256-authvalue.tpm"),
		},
		{
			name: "sealed key",
			f:    mustOpen("./testdata/skey.tpm"),
		},
	} {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			k, err := Decode(tt.f)
			if err != nil {
				t.Fatalf("failed parsing: %v", err)
			}

			if !bytes.Equal(k.Bytes(), tt.f) {
				t.Fatalf("not equal")
			}
		})
	}
}

func must2BPrivate(data []byte) tpm2.TPM2BPrivate {
	return tpm2.TPM2BPrivate{
		Buffer: data,
	}
}

func TestEncodeDecode(t *testing.T) {
	for n, tt := range []struct {
		*TPMKey
	}{
		{
			&TPMKey{
				Keytype:     OIDLoadableKey,
				EmptyAuth:   true,
				Description: "test",
				Parent:      tpm2.TPMHandle(0x40000001),
				Pubkey:      tpm2.New2B(tpm2.ECCSRKTemplate),
				Privkey:     must2BPrivate([]byte("some data")),
			},
		},
	} {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			key, err := Decode(tt.TPMKey.Bytes())
			if err != nil {
				t.Fatalf("failed decoding key: %v", err)
			}
			if !tt.TPMKey.Keytype.Equal(key.Keytype) {
				t.Fatalf("tpmkey keytype is not equal")
			}

			if tt.TPMKey.EmptyAuth != key.EmptyAuth {
				t.Fatalf("tpmkey emptyAuth is not equal")
			}

			if tt.TPMKey.Description != key.Description {
				t.Fatalf("tpmkey description is not equal")
			}

			if tt.TPMKey.Parent != key.Parent {
				t.Fatalf("tpmkey parent is not equal")
			}

			if !bytes.Equal(tpm2.Marshal(tt.TPMKey.Pubkey), tpm2.Marshal(key.Pubkey)) {
				t.Fatalf("tpmkey pubkey is not equal")
			}

			if !bytes.Equal(tt.TPMKey.Privkey.Buffer, key.Privkey.Buffer) {
				t.Fatalf("tpmkey pivkey is not equal")
			}
		})
	}
}
