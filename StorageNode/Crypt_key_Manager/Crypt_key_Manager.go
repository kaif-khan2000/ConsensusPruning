// package Crypt_key_Manager

// import (
// 	"crypto/ecdsa"
// 	"crypto/elliptic"
// 	"crypto/rand"
// 	"crypto/sha256"
// 	"encoding/json"
// 	"fmt"
// 	"math/big"
// )

// // create ecdsa private and public key
// func GenerateKeyPair() (*ecdsa.PrivateKey, ecdsa.PublicKey) {
// 	// generate private key
// 	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// generate public key
// 	publicKey := privateKey.PublicKey
// 	return privateKey, publicKey
// }

// func SerializePrivateKey(privateKey *ecdsa.PrivateKey) []byte {
// 	return elliptic.Marshal(privateKey.Curve, privateKey.X, privateKey.Y)
// }

// func SerializePublicKey(publicKey ecdsa.PublicKey) []byte {
// 	return elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)
// 	// data, err := json.Marshal(publicKey)
// 	// if err != nil {
// 	// 	panic(err)
// 	// }
// 	// return data
// }

// func DeserializePrivateKey(data []byte) *ecdsa.PrivateKey {
// 	curve := elliptic.P256()
// 	x, y := elliptic.Unmarshal(curve, data)
// 	privateKey := new(ecdsa.PrivateKey)
// 	privateKey.Curve = curve
// 	privateKey.X = x
// 	privateKey.Y = y
// 	return privateKey
// }

// func DeserializePublicKey(data []byte) ecdsa.PublicKey {
// 	curve := elliptic.P256()
// 	x, y := elliptic.Unmarshal(curve, data)
// 	publicKey := ecdsa.PublicKey{Curve: curve, X: x, Y: y}
// 	return publicKey
// 	// var publicKey ecdsa.PublicKey
// 	// err := json.Unmarshal([]byte(data), &publicKey)
// 	// if err != nil {
// 	// 	panic(err)
// 	// }
// 	// return publicKey
// }

// // func Verify(publicKey ecdsa.PublicKey, data []byte, signature []byte) bool {
// // 	r := new(big.Int).SetBytes(signature[:len(signature)/2])
// // 	s := new(big.Int).SetBytes(signature[len(signature)/2:])
// // 	return ecdsa.Verify(&publicKey, data, r, s)
// // }

// // verifying the signature of the data with the public key
// func Verify(serializedPublicKey [65]byte, data []byte, signature []byte) bool {
// 	// deserialize public key
// 	publicKey := DeserializePublicKey(serializedPublicKey[:])
// 	// generate r and s parameters
// 	r := new(big.Int).SetBytes(signature[:len(signature)/2])
// 	s := new(big.Int).SetBytes(signature[len(signature)/2:])
// 	// return true or false
// 	res := ecdsa.Verify(&publicKey, data, r, s)
// 	if !res {
// 		fmt.Println("Orphan count: ", publicKey)
// 		serData, _ := json.Marshal(publicKey)
// 		fmt.Println("Orphan count: ", string(serData))
// 	}
// 	return res
// }

// func Sign(privateKey *ecdsa.PrivateKey, data []byte) []byte {
// 	r, s, err := ecdsa.Sign(rand.Reader, privateKey, data)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return append(r.Bytes(), s.Bytes()...)
// }

// func GenerateHashOfObject(p interface{}) [32]byte {
// 	data, _ := json.Marshal(p)
// 	// fmt.Println(string(data))
// 	hash := sha256.Sum256(data)
// 	return hash
// }

// func GenerateHashOfByte(data []byte) [32]byte {
// 	hash := sha256.Sum256(data)
// 	return hash
// }

// func GenerateHashOfString(data string) [32]byte {
// 	hash := sha256.Sum256([]byte(data))
// 	return hash
// }

package Crypt_key_Manager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/json"
	"fmt"
	"math/big"
	"net"

	"github.com/google/uuid"
)

// PointsFromDER Deserialises signature from DER encoded format
func PointsFromDER(der []byte) (*big.Int, *big.Int) {
	R, S := &big.Int{}, &big.Int{}
	data := asn1.RawValue{}
	if _, err := asn1.Unmarshal(der, &data); err != nil {
		panic(err.Error())
	}
	// The format of our DER string is 0x02 + rlen + r + 0x02 + slen + s
	rLen := data.Bytes[1] // The entire length of R + offset of 2 for 0x02 and rlen
	r := data.Bytes[2 : rLen+2]
	// Ignore the next 0x02 and slen bytes and just take the start of S to the end of the byte array
	s := data.Bytes[rLen+4:]
	R.SetBytes(r)
	S.SetBytes(s)
	return R, S
}

// create ecdsa private and public key
func GenerateKeyPair() (*ecdsa.PrivateKey, ecdsa.PublicKey) {
	// generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	// generate public key
	publicKey := privateKey.PublicKey
	return privateKey, publicKey
}

func SerializePrivateKey(privateKey *ecdsa.PrivateKey) []byte {
	return elliptic.Marshal(privateKey.Curve, privateKey.X, privateKey.Y)
}

func SerializePublicKey(publicKey ecdsa.PublicKey) []byte {
	return elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)
	// data, err := json.Marshal(publicKey)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println("Orphan Count:1 data: ", data)
	// fmt.Println("Orphan Count:1 len: ", len(data))
	// fmt.Println("Orphan Count:1 string: ", string(data), len(string(data)))

	// return data
}

func DeserializePrivateKey(data []byte) *ecdsa.PrivateKey {
	curve := elliptic.P256()
	x, y := elliptic.Unmarshal(curve, data)
	privateKey := new(ecdsa.PrivateKey)
	privateKey.Curve = curve
	privateKey.X = x
	privateKey.Y = y
	return privateKey
}

func DeserializePublicKey(data []byte) ecdsa.PublicKey {
	curve := elliptic.P256()
	x, y := elliptic.Unmarshal(curve, data)
	publicKey := ecdsa.PublicKey{Curve: curve, X: x, Y: y}
	return publicKey
	// var publicKey ecdsa.PublicKey

	// err := json.Unmarshal([]byte(data), &publicKey)
	// if err != nil {
	// 	panic(err)
	// }
	// return publicKey
}

// PointsToDER serialises signature to DER encoded format
func PointsToDER(r, s *big.Int) []byte {
	// Ensure the encoded bytes for the r and s values are canonical and
	// thus suitable for DER encoding.
	rb := canonicalizeInt(r)
	sb := canonicalizeInt(s)
	// total length of returned signature is 1 byte for each magic and
	// length (6 total), plus lengths of r and s
	length := 6 + len(rb) + len(sb)
	b := make([]byte, length)
	b[0] = 0x30
	b[1] = byte(length - 2)
	b[2] = 0x02
	b[3] = byte(len(rb))
	offset := copy(b[4:], rb) + 4
	b[offset] = 0x02
	b[offset+1] = byte(len(sb))
	copy(b[offset+2:], sb)
	return b
}

func Sign(privateKey *ecdsa.PrivateKey, hash []byte) []byte {
	// returns the serialized form of the signature
	var signature [72]byte
	if len(hash) != 32 { // check the length of hash
		fmt.Println("Invalid hash")
		return signature[:]
	}
	r, s, _ := ecdsa.Sign(rand.Reader, privateKey, hash)
	copy(signature[:], PointsToDER(r, s))
	return signature[:]
}

func Verify(serializedPublicKey [65]byte, hash []byte, signature []byte) bool {
	PublicKey := DeserializePublicKey(serializedPublicKey[:])
	// fmt.Println(len(signature))
	r, s := PointsFromDER(signature)
	v := ecdsa.Verify(&PublicKey, hash, r, s)
	return v
}

func GenerateHashOfObject(p interface{}) [32]byte {
	data, _ := json.Marshal(p)
	// fmt.Println(string(data))
	hash := sha256.Sum256(data)
	return hash
}

func GenerateHashOfByte(data []byte) [32]byte {
	hash := sha256.Sum256(data)
	return hash
}

func GenerateHashOfString(data string) [32]byte {
	hash := sha256.Sum256([]byte(data))
	return hash
}

func getMacAddr() string {
	ifas, err := net.Interfaces()
	if err != nil {
		return "0"
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as[0]
}

func GenerateSessionId() [32]byte {
	macAddr := getMacAddr()
	id := uuid.New().String()
	hash := GenerateHashOfString(macAddr + id)
	return hash
}

// canonicalizeInt Converts bigint to byte slice
func canonicalizeInt(val *big.Int) []byte {
	b := val.Bytes()
	if len(b) == 0 {
		b = []byte{0x00}
	}
	if b[0]&0x80 != 0 {
		paddedBytes := make([]byte, len(b)+1)
		copy(paddedBytes[1:], b)
		b = paddedBytes
	}
	return b
}
