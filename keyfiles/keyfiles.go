package keyfiles

import (
	encasn1 "encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

var (
	// id-tpmkey OBJECT IDENTIFIER ::=
	//   {joint-iso-itu-t(2) international-organizations(23) 133 10 1}
	// Probably not used, but here for reference
	oidTPMKey = encasn1.ObjectIdentifier{2, 23, 133, 10, 2}

	// id-loadablekey OBJECT IDENTIFIER ::=  {id-tpmkey 3}
	OIDOldLoadableKey = encasn1.ObjectIdentifier{2, 23, 133, 10, 2}

	// id-importablekey OBJECT IDENTIFIER ::=  {id-tpmkey 4}
	OIDLoadableKey = encasn1.ObjectIdentifier{2, 23, 133, 10, 1, 3}

	OIDImportbleKey = encasn1.ObjectIdentifier{2, 23, 133, 10, 1, 4}

	// id-sealedkey OBJECT IDENTIFIER ::= {id-tpmkey 5}
	OIDSealedKey = encasn1.ObjectIdentifier{2, 23, 133, 10, 1, 5}
)

// Errors
var (
	ErrNotTPMKey = errors.New("not a TSS2 PRIVATE KEY")
)

func unwrapPEM(p []byte) ([]byte, error) {
	block, _ := pem.Decode(p)
	if block == nil {
		return nil, fmt.Errorf("not an armored key")
	}
	switch block.Type {
	case "TSS2 PRIVATE KEY":
		return block.Bytes, nil
	default:
		return nil, ErrNotTPMKey
	}
}

type TPMPolicy struct {
	commandCode   int
	commandPolicy []byte
}

func parseTPMPolicy(der *cryptobyte.String) ([]*TPMPolicy, error) {
	var tpmpolicies []*TPMPolicy
	var policy cryptobyte.String
	var hasPolicy bool

	//   policy      [1] EXPLICIT SEQUENCE OF TPMPolicy OPTIONAL,
	if !der.ReadOptionalASN1(&policy, &hasPolicy, asn1.Tag(1).ContextSpecific().Constructed()) {
		return []*TPMPolicy{}, errors.New("should have policy, failed reading")
	}
	if !hasPolicy {
		return []*TPMPolicy{}, nil
	}

	// read outer sequence
	var policySequence cryptobyte.String
	if !policy.ReadASN1(&policySequence, asn1.SEQUENCE) {
		return nil, errors.New("malformed policy sequence")
	}

	if !policySequence.Empty() {
		// TPMPolicy ::= SEQUENCE
		var policyBytes cryptobyte.String
		if !policySequence.ReadASN1(&policyBytes, asn1.SEQUENCE) {
			return nil, errors.New("malformed policy sequence")
		}

		//   commandCode   [0] EXPLICIT INTEGER,
		var ccBytes cryptobyte.String
		if !policyBytes.ReadASN1(&ccBytes, asn1.Tag(0).ContextSpecific().Constructed()) {
			return nil, errors.New("strip tag from commandCode")
		}

		var tpmpolicy TPMPolicy
		if !ccBytes.ReadASN1Integer(&tpmpolicy.commandCode) {
			return nil, errors.New("malformed policy commandCode")
		}

		//   commandPolicy [1] EXPLICIT OCTET STRING
		var cpBytes cryptobyte.String
		if !policyBytes.ReadASN1(&cpBytes, asn1.Tag(1).ContextSpecific().Constructed()) {
			return nil, errors.New("strip tag from commandPolicy")
		}

		if !cpBytes.ReadASN1Bytes(&tpmpolicy.commandPolicy, asn1.OCTET_STRING) {
			return nil, errors.New("malformed policy commandPolicy")
		}
		tpmpolicies = append(tpmpolicies, &tpmpolicy)
	}
	return tpmpolicies, nil
}

type TPMAuthPolicy struct {
	name   string
	policy []*TPMPolicy
}

func parseTPMAuthPolicy(der *cryptobyte.String) ([]*TPMAuthPolicy, error) {
	var authPolicy []*TPMAuthPolicy
	var sAuthPolicy cryptobyte.String
	var hasAuthPolicy bool

	//   authPolicy  [3] EXPLICIT SEQUENCE OF TPMAuthPolicy OPTIONAL,
	if !der.ReadOptionalASN1(&sAuthPolicy, &hasAuthPolicy, asn1.Tag(3).ContextSpecific().Constructed()) {
		return nil, errors.New("should have authPolicy, failed reading")
	}
	if !hasAuthPolicy {
		return authPolicy, nil
	}

	// read outer sequence
	var authPolicySequence cryptobyte.String
	if !sAuthPolicy.ReadASN1(&authPolicySequence, asn1.SEQUENCE) {
		return nil, errors.New("malformed auth policy sequence")
	}

	if !authPolicySequence.Empty() {
		// TPMAuthPolicy ::= SEQUENCE
		var authPolicyBytes cryptobyte.String
		if !authPolicySequence.ReadASN1(&authPolicyBytes, asn1.SEQUENCE) {
			return nil, errors.New("malformed auth policy sequence")
		}

		var tpmAuthPolicy TPMAuthPolicy

		//   name    [0] EXPLICIT UTF8String OPTIONAL,
		var nameBytes cryptobyte.String
		var hasName bool
		if !authPolicyBytes.ReadOptionalASN1(&nameBytes, &hasName, asn1.Tag(0).ContextSpecific().Constructed()) {
			return nil, errors.New("strip tag from commandCode")
		}
		if hasName {
			var nameB cryptobyte.String
			if !nameBytes.ReadASN1(&nameB, asn1.UTF8String) {
				return nil, errors.New("malformed utf8string in auth policy name")
			}
			if !utf8.Valid(nameB) {
				return nil, errors.New("invalid utf8 bytes in name of auth policy")
			}
			tpmAuthPolicy.name = string(nameB)
		}

		//   policy  [1] EXPLICIT SEQUENCE OF TPMPolicy
		tpmpolicies, err := parseTPMPolicy(&authPolicyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed parsing tpm policies in auth policy: %v", err)
		}
		if len(tpmpolicies) == 0 {
			return nil, errors.New("tpm policies in auth policy is empty")
		}
		tpmAuthPolicy.policy = tpmpolicies
	}

	return nil, nil
}

type TPMKey struct {
	keytype    encasn1.ObjectIdentifier
	emptyAuth  bool
	policy     []*TPMPolicy
	secret     []byte
	authPolicy []*TPMAuthPolicy
	Parent     int
	Pubkey     []byte
	Privkey    []byte
}

func Parse(b []byte) (*TPMKey, error) {
	var tkey TPMKey
	var err error

	b, err = unwrapPEM(b)
	if err != nil {
		return nil, err
	}

	// TPMKey ::= SEQUENCE
	s := cryptobyte.String(b)
	if !s.ReadASN1(&s, asn1.SEQUENCE) {
		return nil, errors.New("no sequence")
	}

	//   type        TPMKeyType,
	var oid encasn1.ObjectIdentifier
	if !s.ReadASN1ObjectIdentifier(&oid) {
		return nil, errors.New("no contentinfo oid")
	}

	// Check if we know the keytype
	// TPMKeyType ::= OBJECT IDENTIFIER (
	//   id-loadablekey |
	//   id-importablekey |
	//   id-sealedkey
	// )
	switch {
	case oid.Equal(OIDLoadableKey):
		fallthrough
	case oid.Equal(OIDImportbleKey):
		fallthrough
	case oid.Equal(OIDSealedKey):
		fallthrough
	case oid.Equal(OIDOldLoadableKey):
		tkey.keytype = oid
	default:
		return nil, errors.New("unknown key type")
	}

	//   emptyAuth   [0] EXPLICIT BOOLEAN OPTIONAL,
	var emptyAuth cryptobyte.String
	var hasEmptyAuth bool
	if !s.ReadOptionalASN1(&emptyAuth, &hasEmptyAuth, asn1.Tag(0).ContextSpecific().Constructed()) {
		return nil, errors.New("should have emptyAuth, failed reading")
	}
	if hasEmptyAuth {
		var auth bool
		if !emptyAuth.ReadASN1Boolean(&auth) {
			return nil, errors.New("no emptyAuth bool")
		}
		tkey.emptyAuth = auth
	}

	policy, err := parseTPMPolicy(&s)
	if err != nil {
		return nil, fmt.Errorf("failed reading TPMPolicy: %v", err)
	}
	tkey.policy = policy

	//   secret      [2] EXPLICIT OCTET STRING OPTIONAL,
	var sSecret cryptobyte.String
	var hasSecret bool
	if !s.ReadOptionalASN1(&sSecret, &hasSecret, asn1.Tag(2).ContextSpecific().Constructed()) {
		return nil, errors.New("should have secret, failed reading")
	}
	if hasSecret {
		var secret cryptobyte.String
		if !sSecret.ReadASN1(&secret, asn1.OCTET_STRING) {
			return nil, errors.New("could not parse secret")
		}
		tkey.secret = secret
	}

	authPolicy, err := parseTPMAuthPolicy(&s)
	if err != nil {
		return nil, fmt.Errorf("failed reading TPMAuthPolicy: %v", err)
	}
	tkey.authPolicy = authPolicy

	//   parent      INTEGER,
	var parent int
	if !s.ReadASN1Integer(&parent) {
		return nil, errors.New("failed reading parent")
	}
	tkey.Parent = parent

	//   pubkey      OCTET STRING,
	var pubkey cryptobyte.String
	if !s.ReadASN1(&pubkey, asn1.OCTET_STRING) {
		return nil, errors.New("could not parse pubkey")
	}
	tkey.Pubkey = pubkey

	//   privkey     OCTET STRING
	var privkey cryptobyte.String
	if !s.ReadASN1(&privkey, asn1.OCTET_STRING) {
		return nil, errors.New("could not parse privkey")
	}
	tkey.Privkey = privkey

	return &tkey, nil
}

// TPMPolicy ::= SEQUENCE {
//   commandCode   [0] EXPLICIT INTEGER,
//   commandPolicy [1] EXPLICIT OCTET STRING
// }

// }
