package Crypt_key_Manager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net"

	"github.com/google/uuid"
)

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
}

func Sign(privateKey *ecdsa.PrivateKey, data []byte) []byte {
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, data)
	if err != nil {
		panic(err)
	}
	return append(r.Bytes(), s.Bytes()...)
}

func Verify(serializedPublicKey [65]byte, data []byte, signature []byte) bool {
	// deserialize public key
	publicKey := DeserializePublicKey(serializedPublicKey[:])
	// generate r and s parameters
	r := new(big.Int).SetBytes(signature[:len(signature)/2])
	s := new(big.Int).SetBytes(signature[len(signature)/2:])
	// return true or false
	return ecdsa.Verify(&publicKey, data, r, s)
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
